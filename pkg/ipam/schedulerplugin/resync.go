/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package schedulerplugin

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

type resyncObj struct {
	keyObj *util.KeyObj
	fip    floatingip.FloatingIP
}

// resyncPod releases ips from
// 1. deleted pods whose parent app does not exist
// 2. deleted pods whose parent deployment or statefulset exist but is not ip immutable
// 3. deleted pods whose parent deployment no need so many ips
// 4. deleted pods whose parent statefulset/tapp exist but pod index > .spec.replica
// 5. existing pods but its status is evicted
func (p *FloatingIPPlugin) resyncPod() error {
	glog.V(4).Infof("resync pods+")
	defer glog.V(4).Infof("resync pods-")
	resyncMeta := &resyncMeta{}
	if err := p.fetchChecklist(resyncMeta); err != nil {
		return err
	}
	p.resyncAllocatedIPs(resyncMeta)
	return nil
}

type resyncMeta struct {
	allocatedIPs []resyncObj // allocated ips from galaxy pool
}

func (p *FloatingIPPlugin) fetchChecklist(meta *resyncMeta) error {
	all, err := p.ipam.ByPrefix("")
	if err != nil {
		return err
	}
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
		if fip.PodUid == "" && fip.NodeName == "" && !keyObj.Deployment() &&
			constant.ReleasePolicy(fip.Policy) == constant.ReleasePolicyNever {
			// skip endless checking pod exists for never policy
			continue
		}
		meta.allocatedIPs = append(meta.allocatedIPs, resyncObj{keyObj: keyObj, fip: fip.FloatingIP})
	}
	return nil
}

// #lizard forgives
func (p *FloatingIPPlugin) resyncAllocatedIPs(meta *resyncMeta) {
	for _, obj := range meta.allocatedIPs {
		key := obj.keyObj.KeyInDB
		func() {
			defer p.lockPod(obj.keyObj.PodName, obj.keyObj.Namespace)()
			// we are holding the pod's lock, query again in case the ip has been reallocated.
			fip, err := p.ipam.ByIP(obj.fip.IP)
			if err != nil {
				glog.Warning(err)
				return
			}
			if fip.Key != obj.fip.Key {
				// if key changed, abort
				return
			}
			obj.fip = fip
			running, reason := p.podRunning(obj.keyObj.PodName, obj.keyObj.Namespace, obj.fip.PodUid)
			if running {
				return
			}
			glog.Infof("%s is not running, %s", obj.keyObj.KeyInDB, reason)
			if p.cloudProvider != nil && obj.fip.NodeName != "" {
				// For tapp and sts pod, nodeName will be updated to empty after unassigning
				glog.Infof("UnAssignIP nodeName %s, ip %s, key %s during resync", obj.fip.NodeName,
					obj.fip.IP.String(), key)
				if err := p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
					NodeName:  obj.fip.NodeName,
					IPAddress: obj.fip.IP.String(),
				}); err != nil {
					glog.Warningf("failed to unassign ip %s to %s: %v", obj.fip.IP.String(), key, err)
					// return to retry unassign ip in the next resync loop
					return
				}
				// for tapp and sts pod, we need to clean its node attr and uid
				if err := p.reserveIP(key, key, "unassign ip during resync"); err != nil {
					glog.Error(err)
				}
			}
			releasePolicy := constant.ReleasePolicy(obj.fip.Policy)
			if !obj.keyObj.Deployment() {
				if err := p.unbindNoneDpPod(obj.keyObj, releasePolicy, "during resync"); err != nil {
					glog.Error(err)
				}
				return
			}
			if err := p.unbindDpPod(obj.keyObj, releasePolicy, "during resync"); err != nil {
				glog.Error(err)
			}
		}()
	}
}

func (p *FloatingIPPlugin) podRunning(podName, namespace, podUid string) (bool, string) {
	if podName == "" || namespace == "" {
		return false, ""
	}
	pod, err := p.PodLister.Pods(namespace).Get(podName)
	running, reason1 := runningAndUidMatch(podUid, pod, err)
	if running {
		return true, ""
	}
	// double check with apiserver to confirm it is not running
	pod, err = p.Client.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	running, reason2 := runningAndUidMatch(podUid, pod, err)
	if running {
		return true, ""
	}
	return false, "from podlist: " + reason1 + ", from client: " + reason2
}

func runningAndUidMatch(storedUid string, pod *corev1.Pod, err error) (bool, string) {
	if err != nil {
		if metaErrs.IsNotFound(err) {
			return false, "pod not found"
		}
		// we cannot figure out whether pod exist or not, we'd better keep the ip
		return true, ""
	}
	if storedUid != "" && storedUid != string(pod.GetUID()) {
		return false, fmt.Sprintf("pod current uid %s missmatch stored uid %s", string(pod.GetUID()), storedUid)
	}
	if !finished(pod) {
		return true, ""
	} else {
		return false, "pod finished"
	}
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

// #lizard forgives
// syncPodIP sync pod ip with ipam, if the pod has ipinfos annotation and the ip is unallocated in ipam, allocate the ip
// to the pod
func (p *FloatingIPPlugin) syncPodIP(pod *corev1.Pod) error {
	if pod.Status.Phase != corev1.PodRunning {
		return nil
	}
	if pod.Annotations == nil {
		return nil
	}
	defer p.lockPod(pod.Name, pod.Namespace)()
	keyObj, err := util.FormatKey(pod)
	if err != nil {
		glog.V(5).Infof("sync pod %s/%s ip formatKey with error %v", pod.Namespace, pod.Name, err)
		return nil
	}
	cniArgs, err := constant.UnmarshalCniArgs(pod.Annotations[constant.ExtendedCNIArgsAnnotation])
	if err != nil {
		return err
	}
	ipInfos := cniArgs.Common.IPInfos
	for i := range ipInfos {
		if ipInfos[i].IP == nil || ipInfos[i].IP.IP == nil {
			continue
		}
		if err := p.syncIP(keyObj.KeyInDB, ipInfos[i].IP.IP, pod); err != nil {
			glog.Warningf("sync pod %s ip %s: %v", keyObj.KeyInDB, ipInfos[i].IP.IP.String(), err)
		}
	}
	return nil
}

func (p *FloatingIPPlugin) syncIP(key string, ip net.IP, pod *corev1.Pod) error {
	fip, err := p.ipam.ByIP(ip)
	if err != nil {
		return err
	}
	storedKey := fip.Key
	if storedKey != "" {
		if storedKey != key {
			return fmt.Errorf("conflict ip %s found for both %s and %s", ip.String(), key, storedKey)
		}
	} else {
		attr := floatingip.Attr{
			Policy: parseReleasePolicy(&pod.ObjectMeta), NodeName: pod.Spec.NodeName, Uid: string(pod.UID)}
		if err := p.ipam.AllocateSpecificIP(key, ip, attr); err != nil {
			return err
		}
		glog.Infof("updated floatingip %s to key %s", ip.String(), key)
	}
	return nil
}
