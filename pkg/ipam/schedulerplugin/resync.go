package schedulerplugin

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/constant"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/cloudprovider/rpc"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/schedulerplugin/util"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
	"github.com/golang/glog"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	return true
}

type resyncObj struct {
	attr   string
	ip     uint32
	keyObj *util.KeyObj
}

// resyncPod releases ips from
// 1. deleted pods whose parent app does not exist
// 2. deleted pods whose parent deployment or statefulset exist but is not ip immutable
// 3. deleted pods whose parent deployment no need so many ips
// 4. deleted pods whose parent statefulset exist but pod index > *statefulset.spec.replica
// 5. existing pods but its status is evicted
// 6. deleted pods whose parent app's labels doesn't contain network=floatingip
func (p *FloatingIPPlugin) resyncPod(ipam floatingip.IPAM) error {
	if !p.storeReady() {
		return nil
	}
	glog.Infof("resync pods")
	all, err := ipam.ByPrefix("")
	if err != nil {
		return err
	}
	podInDB := make(map[string]resyncObj)
	assignPodsInDB := make(map[string]resyncObj) // syncing ips with cloudprovider
	for _, fip := range all {
		if fip.Key == "" || util.IsIPPoolKey(fip.Key) {
			continue
		}
		keyObj := util.ParseKey(fip.Key)
		if keyObj.AppName == "" {
			glog.Warningf("unexpected key: %s", fip.Key)
			continue
		}
		// we send unassign request to cloud provider for any release policy
		assignPodsInDB[fip.Key] = resyncObj{keyObj: keyObj, attr: fip.Attr, ip: fip.IP}
		if fip.Policy == uint16(constant.ReleasePolicyNever) {
			// never release these ips
			// for deployment, put back to deployment
			// we do nothing for statefulset pod, because we preserve ip according to its pod name
			if keyObj.IsDeployment && keyObj.PodName != "" {
				podInDB[fip.Key] = resyncObj{keyObj: keyObj, attr: fip.Attr}
			}
			// skip if it is a statefulset key and is ReleasePolicyNever
			continue
		}
		podInDB[fip.Key] = resyncObj{keyObj: keyObj, attr: fip.Attr}
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
	if glog.V(4) {
		podMap := make(map[string]string, len(existPods))
		for k, v := range existPods {
			podMap[k] = util.PodName(v)
		}
		glog.V(4).Infof("existPods %v", podMap)
	}
	for key, obj := range assignPodsInDB {
		if _, ok := existPods[key]; ok {
			continue
		}
		var attr Attr
		if err := json.Unmarshal([]byte(obj.attr), &attr); err != nil {
			glog.Errorf("failed to unmarshal attr %s for pod %s: %v", obj.attr, key, err)
			continue
		}
		if attr.NodeName == "" {
			glog.Errorf("empty nodeName for %s in db", key)
			continue
		}
		if err = p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
			NodeName:  attr.NodeName,
			IPAddress: nets.IntToIP(obj.ip).String(),
		}); err != nil {
			// delete this record from podInDB map to have a retry
			delete(podInDB, key)
			glog.Warningf("failed to unassign ip %s to %s: %v", nets.IntToIP(obj.ip).String(), key, err)
		}
	}
	for key, obj := range podInDB {
		if _, ok := existPods[key]; ok {
			continue
		}
		appFullName := util.Join(obj.keyObj.AppName, obj.keyObj.Namespace)
		// we can't get labels of not exist pod, so get them from it's ss or deployment
		ss, ok := ssMap[appFullName]
		if ok && !util.IsDeploymentKey(key) {
			if !p.hasResourceName(&ss.Spec.Template.Spec) {
				// 6. deleted pods whose parent app's labels doesn't contain network=floatingip
				if err := releaseIP(ipam, key, deletedAndLabelMissMatchPod); err != nil {
					glog.Warningf("[%s] %v", ipam.Name(), err)
				}
				continue
			}
			if parseReleasePolicy(&ss.Spec.Template.ObjectMeta) != constant.ReleasePolicyImmutable {
				// 2. deleted pods whose parent statefulset exist but is not ip immutable
				if err := releaseIP(ipam, key, deletedAndIPMutablePod); err != nil {
					glog.Warningf("[%s] %v", ipam.Name(), err)
				}
				continue
			}
			index, err := parsePodIndex(key)
			if err != nil {
				glog.Errorf("invalid pod name %s of ss %s: %v", key, util.StatefulsetName(ss), err)
				continue
			}
			if ss.Spec.Replicas != nil && *ss.Spec.Replicas < int32(index)+1 {
				if err := releaseIP(ipam, key, deletedAndIPMutablePod); err != nil {
					glog.Warningf("[%s] %v", ipam.Name(), err)
				}
				continue
			}
			continue
		}
		dp, ok := dpMap[appFullName]
		if ok && util.IsDeploymentKey(key) {
			if !p.hasResourceName(&dp.Spec.Template.Spec) {
				// 6. deleted pods whose parent app's labels doesn't contain network=floatingip
				if err := releaseIP(ipam, key, deletedAndLabelMissMatchPod); err != nil {
					glog.Warningf("[%s] %v", ipam.Name(), err)
				}
				continue
			}
			policy := parseReleasePolicy(&dp.Spec.Template.ObjectMeta)
			if policy == constant.ReleasePolicyPodDelete {
				// 2. deleted pods whose parent deployment exist but is not ip immutable
				if err := releaseIP(ipam, key, deletedAndIPMutablePod); err != nil {
					glog.Warningf("[%s] %v", ipam.Name(), err)
				}
				continue
			}
			poolPrefix := obj.keyObj.PoolPrefix()
			fips, err := ipam.ByPrefix(poolPrefix)
			if err != nil {
				glog.Errorf("failed query prefix: %v", err)
				continue
			}
			replicas := int(*dp.Spec.Replicas)
			if replicas < len(fips) && policy == constant.ReleasePolicyImmutable {
				if err = releaseIP(ipam, key, deletedAndScaledDownDpPod); err != nil {
					glog.Errorf("[%s] %v", ipam.Name(), err)
				}
			} else if poolPrefix != key {
				// if this is not a pool key, i.e. a pod key, update it to a pool key
				if err = ipam.UpdateKey(key, poolPrefix); err != nil {
					glog.Errorf("failed reserver deployment %s ip: %v", poolPrefix, err)
				}
			}
			continue
		} else if util.IsDeploymentKey(key) {
			fip, err := ipam.First(key)
			if err != nil {
				glog.Errorf("failed get key %s: %v", key, err)
				continue
			} else if fip == nil {
				continue
			}
			if fip.FIP.Policy == uint16(constant.ReleasePolicyNever) {
				var attr Attr
				if err := json.Unmarshal([]byte(obj.attr), &attr); err != nil {
					glog.Errorf("failed to unmarshal attr %s for pod %s: %v", obj.attr, key, err)
					continue
				}
				keyObj := util.ParseKey(fip.FIP.Key)
				if err = ipam.UpdateKey(key, keyObj.PoolPrefix()); err != nil {
					glog.Errorf("failed reserve fip: %v", err)
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
	glog.V(4).Infof("%v", key2App)
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
	glog.V(4).Infof("%v", key2App)
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
	glog.Infof("sync pod ips into DB")
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
		if err := ipam.AllocateSpecificIP(key, ip, parseReleasePolicy(&pod.ObjectMeta), getAttr(pod, pod.Spec.NodeName)); err != nil {
			return err
		}
		glog.Infof("[%s] updated floatingip %s to key %s", ipam.Name(), ip.String(), key)
	}
	return nil
}
