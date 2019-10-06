package schedulerplugin

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	tappv1 "git.code.oa.com/gaia/tapp-controller/pkg/apis/tappcontroller/v1alpha1"
	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/constant"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/cloudprovider/rpc"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/schedulerplugin/util"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/database"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/nets"
	"github.com/golang/glog"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (p *FloatingIPPlugin) storeReady() bool {
	if !p.PodHasSynced() {
		glog.V(3).Infof("the pod store has not been synced yet")
		return false
	}
	if !p.StatefulSetSynced() {
		glog.V(3).Infof("the statefulset store has not been synced yet")
		return false
	}
	if !p.DeploymentSynced() {
		glog.V(3).Infof("the deployment store has not been synced yet")
		return false
	}
	if !p.TAppHasSynced() {
		glog.V(3).Infof("the tapp store has not been synced yet")
		return false
	}
	if !p.PoolSynced() {
		glog.V(3).Infof("the pool store has not been synced yet")
		return false
	}
	return true
}

type resyncObj struct {
	keyObj *util.KeyObj
	fip    database.FloatingIP
}

// resyncPod releases ips from
// 1. deleted pods whose parent app does not exist
// 2. deleted pods whose parent deployment or statefulset exist but is not ip immutable
// 3. deleted pods whose parent deployment no need so many ips
// 4. deleted pods whose parent statefulset/tapp exist but pod index > .spec.replica
// 5. existing pods but its status is evicted
func (p *FloatingIPPlugin) resyncPod(ipam floatingip.IPAM) error {
	if !p.storeReady() {
		return nil
	}
	glog.V(4).Infof("resync pods")
	all, err := ipam.ByPrefix("")
	if err != nil {
		return err
	}
	allocatedIPs := make(map[string]resyncObj)
	assignedPods := make(map[string]resyncObj) // syncing ips with cloudprovider
	for i := range all {
		fip := all[i]
		if fip.Key == "" {
			continue
		}
		keyObj := util.ParseKey(fip.Key)
		if keyObj.PodName == "" {
			continue
		}
		if keyObj.AppName == "" {
			glog.Warningf("unexpected key: %s", fip.Key)
			continue
		}
		if p.cloudProvider != nil {
			// we send unassign request to cloud provider for any release policy
			assignedPods[fip.Key] = resyncObj{keyObj: keyObj, fip: fip}
		}
		if fip.Policy == uint16(constant.ReleasePolicyNever) {
			// never release these ips
			// for deployment, put back to deployment
			// we do nothing for statefulset or tapp pod, because we preserve ip according to its pod name
			if keyObj.Deployment() {
				allocatedIPs[fip.Key] = resyncObj{keyObj: keyObj, fip: fip}
			}
			// skip if it is a statefulset key and is ReleasePolicyNever
			continue
		}
		allocatedIPs[fip.Key] = resyncObj{keyObj: keyObj, fip: fip}
	}
	pods, err := p.listWantedPods()
	if err != nil {
		return err
	}
	existPods := map[string]*corev1.Pod{}
	for i := range pods {
		if evicted(pods[i]) {
			// for evicted pod, treat as not exist
			continue
		}
		keyObj := util.FormatKey(pods[i])
		existPods[keyObj.KeyInDB] = pods[i]
	}
	ssMap, err := p.getSSMap()
	if err != nil {
		return err
	}
	dpMap, err := p.getDPMap()
	if err != nil {
		return err
	}
	tappMap, err := p.getTAppMap()
	if err != nil {
		return err
	}
	if glog.V(5) {
		podMap := make(map[string]string, len(existPods))
		for k, v := range existPods {
			podMap[k] = util.PodName(v)
		}
		glog.V(5).Infof("existPods %v", podMap)
	}
	if p.cloudProvider != nil {
		for key, obj := range assignedPods {
			if _, ok := existPods[key]; ok {
				continue
			}
			// check with apiserver to confirm it really not exist
			if p.podExist(obj.keyObj.PodName, obj.keyObj.Namespace) {
				continue
			}
			var attr Attr
			if err := json.Unmarshal([]byte(obj.fip.Attr), &attr); err != nil {
				glog.Errorf("failed to unmarshal attr %s for pod %s: %v", obj.fip.Attr, key, err)
				continue
			}
			if attr.NodeName == "" {
				glog.Errorf("empty nodeName for %s in db", key)
				continue
			}
			glog.Infof("UnAssignIP nodeName %s, ip %s, key %s during resync", attr.NodeName, nets.IntToIP(obj.fip.IP).String(), key)
			if err = p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
				NodeName:  attr.NodeName,
				IPAddress: nets.IntToIP(obj.fip.IP).String(),
			}); err != nil {
				// delete this record from allocatedIPs map to have a retry
				delete(allocatedIPs, key)
				glog.Warningf("failed to unassign ip %s to %s: %v", nets.IntToIP(obj.fip.IP).String(), key, err)
				continue
			}
			if err := ipam.ReserveIP(key, key, getAttr("")); err != nil {
				glog.Errorf("failed to reserve %s ip: %v", key, err)
			}
		}
	}
	for key, obj := range allocatedIPs {
		if _, ok := existPods[key]; ok {
			continue
		}
		// check with apiserver to confirm it really not exist
		if p.podExist(obj.keyObj.PodName, obj.keyObj.Namespace) {
			continue
		}
		appFullName := util.Join(obj.keyObj.AppName, obj.keyObj.Namespace)
		releasePolicy := constant.ReleasePolicy(obj.fip.Policy)
		// we can't get labels of not exist pod, so get them from it's ss or deployment
		if !obj.keyObj.Deployment() {
			var appExist bool
			var replicas int32
			if obj.keyObj.StatefulSet() {
				ss, ok := ssMap[appFullName]
				if ok {
					appExist = true
					replicas = 1
					if ss.Spec.Replicas != nil {
						replicas = *ss.Spec.Replicas
					}
				}
			} else if obj.keyObj.TApp() {
				tapp, ok := tappMap[appFullName]
				if ok {
					appExist = true
					replicas = tapp.Spec.Replicas
				}
			} else {
				glog.Warningf("unknow app type of key %s", obj.keyObj)
				continue
			}
			if should, reason := p.shouldReleaseDuringResync(obj.keyObj, releasePolicy, appExist, replicas); should {
				if err := releaseIP(ipam, key, reason); err != nil {
					glog.Warningf("[%s] %v", ipam.Name(), err)
				}
			}
			continue
		}
		dp, ok := dpMap[appFullName]
		if ok {
			if err := unbindDpPod(key, obj.keyObj.PoolPrefix(), ipam, p.dpLockPool, int(*dp.Spec.Replicas), releasePolicy, "resyncing"); err != nil {
				glog.Error(err)
			}
			continue
		} else {
			fip, err := ipam.First(key)
			if err != nil {
				glog.Errorf("failed get key %s: %v", key, err)
				continue
			} else if fip == nil {
				continue
			}
			if fip.FIP.Policy == uint16(constant.ReleasePolicyNever) {
				keyObj := util.ParseKey(fip.FIP.Key)
				if err := reserveIP(key, keyObj.PoolPrefix(), ipam, "never release policy during resyncing"); err != nil {
					glog.Error(err)
				}
			} else {
				if err = releaseIP(ipam, key, deletedAndIPMutablePod); err != nil {
					glog.Errorf("failed release ip: %v", err)
				}
			}
			continue
		}
	}
	return nil
}

func (p *FloatingIPPlugin) shouldReleaseDuringResync(keyObj *util.KeyObj, releasePolicy constant.ReleasePolicy,
	parentAppExist bool, replicas int32) (bool, string) {
	if !parentAppExist {
		if releasePolicy != constant.ReleasePolicyNever {
			return true, deletedAndParentAppNotExistPod
		}
		return false, ""
	}
	if releasePolicy != constant.ReleasePolicyImmutable {
		// 2. deleted pods whose parent statefulset or tapp exist but is not ip immutable
		return true, deletedAndIPMutablePod
	}
	index, err := parsePodIndex(keyObj.KeyInDB)
	if err != nil {
		glog.Errorf("invalid pod name of key %s: %v", keyObj.KeyInDB, err)
		return false, ""
	}
	if replicas < int32(index)+1 {
		return true, deletedAndScaledDownAppPod
	}
	return false, ""
}

func (p *FloatingIPPlugin) podExist(podName, namespace string) bool {
	_, err := p.Client.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	if err != nil {
		if metaErrs.IsNotFound(err) {
			return false
		}
		// we cannot figure out whether pod exist or not
	}
	return true
}

func TAppFullName(tapp *tappv1.TApp) string {
	return fmt.Sprintf("%s_%s", tapp.Namespace, tapp.Name)
}

func (p *FloatingIPPlugin) getTAppMap() (map[string]*tappv1.TApp, error) {
	if p.TAppLister == nil {
		return map[string]*tappv1.TApp{}, nil
	}
	tApps, err := p.TAppLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	key2App := make(map[string]*tappv1.TApp)
	for i := range tApps {
		if !p.hasResourceName(&tApps[i].Spec.Template.Spec) {
			continue
		}
		key2App[TAppFullName(tApps[i])] = tApps[i]
	}
	glog.V(5).Infof("%v", key2App)
	return key2App, nil
}

func (p *FloatingIPPlugin) getSSMap() (map[string]*appv1.StatefulSet, error) {
	sss, err := p.StatefulSetLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	key2App := make(map[string]*appv1.StatefulSet)
	for i := range sss {
		if !p.hasResourceName(&sss[i].Spec.Template.Spec) {
			continue
		}
		key2App[util.StatefulsetName(sss[i])] = sss[i]
	}
	glog.V(5).Infof("%v", key2App)
	return key2App, nil
}

func (p *FloatingIPPlugin) getDPMap() (map[string]*appv1.Deployment, error) {
	dps, err := p.DeploymentLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	key2App := make(map[string]*appv1.Deployment)
	for i := range dps {
		if !p.hasResourceName(&dps[i].Spec.Template.Spec) {
			continue
		}
		key2App[util.DeploymentName(dps[i])] = dps[i]
	}
	glog.V(5).Infof("%v", key2App)
	return key2App, nil
}

func parsePodIndex(name string) (int, error) {
	parts := strings.Split(name, "-")
	return strconv.Atoi(parts[len(parts)-1])
}

func (p *FloatingIPPlugin) listWantedPods() ([]*corev1.Pod, error) {
	pods, err := p.PodLister.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %v", err)
	}
	var filtered []*corev1.Pod
	for i := range pods {
		if p.hasResourceName(&pods[i].Spec) {
			filtered = append(filtered, pods[i])
		}
	}
	return filtered, nil
}

// syncPodIPs sync all pods' ips with db, if a pod has PodIP and its ip is unallocated, allocate the ip to it
func (p *FloatingIPPlugin) syncPodIPsIntoDB() {
	glog.V(4).Infof("sync pod ips into DB")
	if !p.storeReady() {
		return
	}
	pods, err := p.listWantedPods()
	if err != nil {
		glog.Warning(err)
		return
	}
	for i := range pods {
		if err := p.syncPodIP(pods[i]); err != nil {
			glog.Warning(err)
		}
	}
}

// syncPodIP sync pod ip with db, if the pod has ipinfos annotation and the ip is unallocated in db, allocate the ip to the pod
func (p *FloatingIPPlugin) syncPodIP(pod *corev1.Pod) error {
	if pod.Status.Phase != corev1.PodRunning {
		return nil
	}
	if pod.Annotations == nil {
		return nil
	}
	keyObj := util.FormatKey(pod)
	ipInfos, err := constant.ParseIPInfo(pod.Annotations[constant.ExtendedCNIArgsAnnotation])
	if err != nil {
		return err
	}
	if len(ipInfos) == 0 || ipInfos[0].IP == nil {
		// should not happen
		return fmt.Errorf("empty ipinfo for pod %s", keyObj.KeyInDB)
	}
	if err := p.syncIP(p.ipam, keyObj.KeyInDB, ipInfos[0].IP.IP, pod); err != nil {
		return fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	if p.enabledSecondIP(pod) {
		if len(ipInfos) == 1 || ipInfos[1].IP == nil {
			return fmt.Errorf("none second ipinfo for pod %s", keyObj.KeyInDB)
		}
		if err := p.syncIP(p.secondIPAM, keyObj.KeyInDB, ipInfos[1].IP.IP, pod); err != nil {
			return fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
		}
	}
	return nil
}

func (p *FloatingIPPlugin) syncIP(ipam floatingip.IPAM, key string, ip net.IP, pod *corev1.Pod) error {
	fip, err := ipam.ByIP(ip)
	if err != nil {
		return err
	}
	storedKey := fip.Key
	if storedKey != "" {
		if storedKey != key {
			return fmt.Errorf("conflict ip %s found for both %s and %s", ip.String(), key, storedKey)
		}
	} else {
		if err := ipam.AllocateSpecificIP(key, ip, parseReleasePolicy(&pod.ObjectMeta), getAttr(pod.Spec.NodeName)); err != nil {
			return err
		}
		glog.Infof("[%s] updated floatingip %s to key %s", ipam.Name(), ip.String(), key)
	}
	return nil
}
