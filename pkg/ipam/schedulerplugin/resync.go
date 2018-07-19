package schedulerplugin

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	tappv1 "git.code.oa.com/gaia/tapp-controller/pkg/apis/tappcontroller/v1alpha1"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (p *FloatingIPPlugin) storeReady() bool {
	if !p.TAppHasSynced() {
		glog.V(3).Infof("the tapp store has not been synced yet")
		return false
	}
	if !p.PodHasSynced() {
		glog.V(3).Infof("the pod store has not been synced yet")
		return false
	}
	return true
}

// resyncPod releases ips from
// 1. deleted pods whose parent is not tapp
// 2. deleted pods whose parent is an unexisting tapp
// 3. deleted pods whose parent tapp exist but is not ip immutable
// 4. deleted pods whose parent tapp exist but instance status in tapp.spec.statuses = killed
// 5. existing pods but its status is evicted (TApp doesn't have evicted pods)
func (p *FloatingIPPlugin) resyncPod(ipam floatingip.IPAM) error {
	if !p.storeReady() {
		return nil
	}
	glog.Infof("resync pods")
	all, err := ipam.QueryByPrefix("")
	if err != nil {
		return err
	}
	podInDB := make(map[string]string) // podFullName to TappFullName
	for _, key := range all {
		if key == "" {
			continue
		}
		tappName, _, namespace := resolveTAppPodName(key)
		if namespace == "" {
			glog.Warningf("unexpected key: %s", key)
			continue
		}
		podInDB[key] = fmtKey(namespace, tappName)
	}
	pods, err := p.PodLister.List(p.objectSelector)
	if err != nil {
		return err
	}
	existPods := map[string]*corev1.Pod{}
	for i := range pods {
		if evicted(pods[i]) {
			// 5. existing pods but its status is evicted (TApp doesn't have evicted pods, it simply deletes pods which are evicted by kubelet)
			if err := releaseIP(ipam, keyInDB(pods[i]), evicted_pod); err != nil {
				glog.Warningf("[%s] %v", ipam.Name(), err)
			}
		}
		existPods[keyInDB(pods[i])] = pods[i]
	}
	tappMap, err := p.getTAppMap()
	if err != nil {
		return err
	}
	glog.V(4).Infof("existPods %v", existPods)
	for podFullName, tappFullName := range podInDB {
		if _, ok := existPods[podFullName]; ok {
			continue
		}
		// we can't get labels of not exist pod, so get them from it's rs or tapp
		var tapp *tappv1.TApp
		if existTapp, ok := tappMap[tappFullName]; ok {
			tapp = existTapp
		} else {
			// 1. deleted pods whose parent is not tapp
			// 2. deleted pods whose parent is an unexisting tapp
			if err := releaseIP(ipam, podFullName, deleted_and_parent_tapp_not_exist_pod); err != nil {
				glog.Warningf("[%s] %v", ipam.Name(), err)
			}
			continue
		}
		if !p.wantedObject(&tapp.ObjectMeta) {
			continue
		}
		if !p.immutableSeletor.Matches(labels.Set(tapp.GetLabels())) {
			// 3. deleted pods whose parent tapp exist but is not ip immutable
			if err := releaseIP(ipam, podFullName, deleted_and_ip_mutable_pod); err != nil {
				glog.Warningf("[%s] %v", ipam.Name(), err)
			}
			continue
		}
		// ns_tapp-12, "12" = str[(2+1+4+1):]
		podId := podFullName[len(tapp.Namespace)+1+len(tapp.Name)+1:]
		if status := tapp.Spec.Statuses[podId]; tappInstanceKilled(status) {
			// 4. deleted pods whose parent tapp exist but instance status in tapp.spec.statuses = killed
			if err := releaseIP(ipam, podFullName, deleted_and_killed_tapp_pod); err != nil {
				glog.Warningf("[%s] %v", ipam.Name(), err)
			}
		}
	}
	return nil
}

func (p *FloatingIPPlugin) getTAppMap() (map[string]*tappv1.TApp, error) {
	tApps, err := p.TAppLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	app2TApp := make(map[string]*tappv1.TApp)
	for i := range tApps {
		if !p.wantedObject(&tApps[i].ObjectMeta) {
			continue
		}
		app2TApp[TAppFullName(tApps[i])] = tApps[i]
	}
	glog.V(4).Infof("%v", app2TApp)
	return app2TApp, nil
}

func keyInDB(pod *corev1.Pod) string {
	return fmt.Sprintf("%s_%s", pod.Namespace, pod.Name)
}

func fmtKey(name, namespace string) string {
	return fmt.Sprintf("%s_%s", namespace, name)
}

func TAppFullName(tapp *tappv1.TApp) string {
	return fmt.Sprintf("%s_%s", tapp.Namespace, tapp.Name)
}

// resolveTAppPodName returns tappname, podId, namespace
func resolveTAppPodName(podFullName string) (string, string, string) {
	// namespace_tappname-id, e.g. default_fip-0
	// _ is not a valid char in appname
	parts := strings.SplitN(podFullName, "_", 2)
	if len(parts) != 2 {
		return "", "", ""
	}
	lastIndex := strings.LastIndexByte(parts[1], '-')
	if lastIndex == -1 {
		return "", "", ""
	}
	return parts[1][:lastIndex], parts[1][lastIndex+1:], parts[0]
}

func ownerIsTApp(pod *corev1.Pod) bool {
	for i := range pod.OwnerReferences {
		if pod.OwnerReferences[i].Kind == "TApp" {
			return true
		}
	}
	return false
}

// syncPodIPs sync all pods' ips with db, if a pod has PodIP and its ip is unallocated, allocate the ip to it
func (p *FloatingIPPlugin) syncPodIPsIntoDB() {
	glog.Infof("sync pod ips into DB")
	if !p.storeReady() {
		return
	}
	pods, err := p.PodLister.List(p.objectSelector)
	if err != nil {
		glog.Warningf("failed to list pods: %v", err)
		return
	}
	for i := range pods {
		if err := p.syncPodIP(pods[i]); err != nil {
			glog.Warning(err)
		}
	}
}

// syncPodIP sync pod ip with db, if the pod has PodIP and the ip is unallocated in db, allocate the ip to the pod
func (p *FloatingIPPlugin) syncPodIP(pod *corev1.Pod) error {
	if pod.Status.Phase != corev1.PodRunning {
		return nil
	}
	ip := net.ParseIP(pod.Status.PodIP)
	if ip == nil {
		// A binded Pod's IpInfo annotation is lost, we need to find its ip
		// It happens if a node crashes causing pods on it to be relaunched during which we are upgrading gaiastack from 2.6 to 2.8
		if pod.Spec.NodeName != "" {
			return p.syncPodAnnotation(pod)
		}
		return nil
	}
	key := keyInDB(pod)
	if err := p.syncIP(p.ipam, key, ip); err != nil {
		return fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	if p.enabledSecondIP(pod) && pod.Annotations != nil {
		secondIPStr := pod.Annotations[private.AnnotationKeySecondIPInfo]
		var secondIPInfo floatingip.IPInfo
		if err := json.Unmarshal([]byte(secondIPStr), &secondIPInfo); err != nil {
			return fmt.Errorf("failed to unmarshal secondip %s: %v", secondIPStr, err)
		}
		if secondIPInfo.IP == nil {
			return fmt.Errorf("invalid secondip annotation: %s", secondIPStr)
		}
		if err := p.syncIP(p.secondIPAM, key, secondIPInfo.IP.IP); err != nil {
			return fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
		}
	}
	return p.syncPodAnnotation(pod)
}

func (p *FloatingIPPlugin) syncIP(ipam floatingip.IPAM, key string, ip net.IP) error {
	storedKey, err := ipam.QueryByIP(ip)
	if err != nil {
		return err
	}
	if storedKey != "" {
		if storedKey != key {
			return fmt.Errorf("conflict ip %s found for both %s and %s", ip.String(), key, storedKey)
		}
	} else {
		if err := ipam.AllocateSpecificIP(key, ip); err != nil {
			return err
		}
		glog.Infof("[%s] updated floatingip %s to key %s", ipam.Name(), ip.String(), key)
	}
	return nil
}

func (p *FloatingIPPlugin) syncPodAnnotation(pod *corev1.Pod) error {
	key := keyInDB(pod)
	// create ipInfo annotation for gaiastack 2.6 pod
	if pod.Annotations == nil || pod.Annotations[private.AnnotationKeyIPInfo] == "" {
		ipInfo, err := p.ipam.QueryFirst(key)
		if err != nil {
			return fmt.Errorf("failed to query ipInfo of %s", key)
		}
		data, err := json.Marshal(ipInfo)
		if err != nil {
			return fmt.Errorf("failed to marshal ipinfo %v: %v", ipInfo, err)
		}
		m := make(map[string]string)
		m[private.AnnotationKeyIPInfo] = string(data)
		ret := &unstructured.Unstructured{}
		ret.SetAnnotations(m)
		patchData, err := json.Marshal(ret)
		if err != nil {
			glog.Error(err)
		}
		if err := wait.PollImmediate(time.Millisecond*500, 20*time.Second, func() (bool, error) {
			_, err := p.Client.CoreV1().Pods(pod.Namespace).Patch(pod.Name, types.MergePatchType, patchData)
			if err != nil {
				glog.Warningf("failed to update pod %s: %v", key, err)
				return false, nil
			}
			glog.V(3).Infof("updated annotation %s=%s for old pod %s (created by gaiastack 2.6)", private.AnnotationKeyIPInfo, m[private.AnnotationKeyIPInfo], key)
			return true, nil
		}); err != nil {
			// If fails to update, depending on resync to update
			return fmt.Errorf("failed to update pod %s: %v", key, err)
		}
	}
	return nil
}

func (p *FloatingIPPlugin) syncTAppRequestResource() {
	if !p.storeReady() {
		return
	}
	glog.Info("sync TApp request resource")
	tapps, err := p.TAppLister.List(p.objectSelector)
	if err != nil {
		glog.Warningf("failed to list pods: %v", err)
		return
	}
	one := resource.NewQuantity(1, resource.DecimalSI)
	for _, tapp := range tapps {
		fullname := TAppFullName(tapp)
		var needUpdate bool
		for _, container := range tapp.Spec.Template.Spec.Containers {
			if _, ok := container.Resources.Requests[corev1.ResourceName(private.FloatingIPResource)]; !ok {
				needUpdate = true
				break
			}
		}
		if !needUpdate {
			for _, podTemplate := range tapp.Spec.TemplatePool {
				for _, container := range podTemplate.Spec.Containers {
					if _, ok := container.Resources.Requests[corev1.ResourceName(private.FloatingIPResource)]; !ok {
						needUpdate = true
						break
					}
				}
				if needUpdate {
					break
				}
			}
		}
		if !needUpdate {
			continue
		}
		if err := wait.PollImmediate(time.Millisecond*500, 20*time.Second, func() (bool, error) {
			for _, container := range tapp.Spec.Template.Spec.Containers {
				container.Resources.Requests[corev1.ResourceName(private.FloatingIPResource)] = *one
			}
			for _, podTemplate := range tapp.Spec.TemplatePool {
				for _, container := range podTemplate.Spec.Containers {
					container.Resources.Requests[corev1.ResourceName(private.FloatingIPResource)] = *one
				}
			}
			if _, err := p.TAppClient.TappcontrollerV1alpha1().TApps(tapp.Namespace).Update(tapp); err != nil {
				glog.Warningf("failed to update tapp resource %s: %v", fullname, err)
				return false, err
			}
			return true, nil
		}); err != nil {
			// If fails to update, depending on resync to update
			glog.Warningf("failed to update tapp resource %s: %v", fullname, err)
		}
	}
}

// labelSubnet labels node which have floatingip configuration with labels network=floatingip
// TODO After we finally remove all old pods created by previous gaiastack which has network=floatingip node selector
// we can remove all network=floatingip label from galaxy code
func (p *FloatingIPPlugin) labelNodes() {
	if p.conf != nil && p.conf.DisableLabelNode {
		return
	}
	nodes, err := p.Client.CoreV1().Nodes().List(v1.ListOptions{})
	if err != nil {
		glog.Warningf("failed to get nodes: %v", err)
		return
	}
	for i := range nodes.Items {
		node := nodes.Items[i]
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		if node.Labels[private.LabelKeyNetworkType] == private.NodeLabelValueNetworkTypeFloatingIP {
			continue
		}
		_, err := p.getNodeSubnet(&node)
		if err != nil {
			// node has no fip configuration
			return
		}
		node.Labels[private.LabelKeyNetworkType] = private.NodeLabelValueNetworkTypeFloatingIP
		_, err = p.Client.CoreV1().Nodes().Update(&node)
		if err != nil {
			glog.Warningf("failed to update node label: %v", err)
		} else {
			glog.Infof("update node %s label %s=%s", node.Name, private.LabelKeyNetworkType, private.NodeLabelValueNetworkTypeFloatingIP)
		}
	}
}
