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
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	tappv1 "tkestack.io/tapp/pkg/apis/tappcontroller/v1"
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
	resyncMeta := &resyncMeta{
		allocatedIPs: make(map[string]resyncObj),
	}
	if err := p.fetchChecklist(resyncMeta); err != nil {
		return err
	}
	if err := p.fetchAppAndPodMeta(resyncMeta); err != nil {
		return err
	}
	p.resyncAllocatedIPs(resyncMeta)
	return nil
}

type resyncMeta struct {
	allocatedIPs map[string]resyncObj // allocated ips from galaxy pool
	existPods    map[string]*corev1.Pod
	tappMap      map[string]*tappv1.TApp
	ssMap        map[string]*appv1.StatefulSet
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
		meta.allocatedIPs[fip.Key] = resyncObj{keyObj: keyObj, fip: fip}
	}
	return nil
}

func (p *FloatingIPPlugin) fetchAppAndPodMeta(meta *resyncMeta) error {
	var err error
	meta.existPods, err = p.listWantedPodsToMap()
	if err != nil {
		return err
	}
	meta.ssMap, err = p.getSSMap()
	if err != nil {
		return err
	}
	meta.tappMap, err = p.getTAppMap()
	if err != nil {
		return err
	}
	return nil
}

// #lizard forgives
func (p *FloatingIPPlugin) resyncAllocatedIPs(meta *resyncMeta) {
	for key, obj := range meta.allocatedIPs {
		func() {
			defer p.lockPod(obj.keyObj.PodName, obj.keyObj.Namespace)()
			if _, ok := meta.existPods[key]; ok {
				return
			}
			// check with apiserver to confirm it really not exist
			if p.podExist(obj.keyObj.PodName, obj.keyObj.Namespace) {
				return
			}
			if p.cloudProvider != nil {
				var attr Attr
				if err := json.Unmarshal([]byte(obj.fip.Attr), &attr); err != nil {
					glog.Errorf("failed to unmarshal attr %s for pod %s: %v", obj.fip.Attr, key, err)
					return
				}
				// For tapp and sts pod, nodeName will be updated to empty after unassigning
				if attr.NodeName != "" {
					glog.Infof("UnAssignIP nodeName %s, ip %s, key %s during resync", attr.NodeName,
						obj.fip.IP.String(), key)
					if err := p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
						NodeName:  attr.NodeName,
						IPAddress: obj.fip.IP.String(),
					}); err != nil {
						glog.Warningf("failed to unassign ip %s to %s: %v", obj.fip.IP.String(), key, err)
						// return to retry unassign ip in the next resync loop
						return
					}
					// for tapp and sts pod, we need to clean its node attr and uid
					if err := p.ipam.ReserveIP(key, key, getAttr("", "")); err != nil {
						glog.Errorf("failed to reserve %s ip: %v", key, err)
					}
				}
			}
			releasePolicy := constant.ReleasePolicy(obj.fip.Policy)
			// we can't get labels of not exist pod, so get them from it's ss or deployment
			if !obj.keyObj.Deployment() {
				p.resyncTappOrSts(meta, obj.keyObj, releasePolicy)
				return
			}
			if err := p.unbindDpPod(obj.keyObj, releasePolicy, "during resyncing"); err != nil {
				glog.Error(err)
			}
		}()
	}
}

func (p *FloatingIPPlugin) resyncTappOrSts(meta *resyncMeta, keyObj *util.KeyObj, releasePolicy constant.ReleasePolicy) {
	if releasePolicy == constant.ReleasePolicyNever {
		return
	}
	var appExist bool
	var replicas int32
	appFullName := util.Join(keyObj.AppName, keyObj.Namespace)
	if keyObj.StatefulSet() {
		ss, ok := meta.ssMap[appFullName]
		if ok {
			appExist = true
			replicas = 1
			if ss.Spec.Replicas != nil {
				replicas = *ss.Spec.Replicas
			}
		}
	} else if keyObj.TApp() {
		tapp, ok := meta.tappMap[appFullName]
		if ok {
			appExist = true
			replicas = tapp.Spec.Replicas
		}
	} else {
		// release for other apps
		appExist = false
	}
	if should, reason, err := p.shouldRelease(keyObj, releasePolicy, appExist, replicas); err != nil {
		glog.Warning(err)
	} else if should {
		if err := p.releaseIP(keyObj.KeyInDB, fmt.Sprintf("%s during resyncing", reason)); err != nil {
			glog.Warning(err)
		}
	}
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

func (p *FloatingIPPlugin) listWantedPodsToMap() (map[string]*corev1.Pod, error) {
	pods, err := p.listWantedPods()
	if err != nil {
		return nil, err
	}
	existPods := map[string]*corev1.Pod{}
	for i := range pods {
		if evicted(pods[i]) {
			// for evicted pod, treat as not exist
			continue
		}
		keyObj, err := util.FormatKey(pods[i])
		if err != nil {
			continue
		}
		existPods[keyObj.KeyInDB] = pods[i]
	}
	return existPods, nil
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
// syncPodIP sync pod ip with db, if the pod has ipinfos annotation and the ip is unallocated in db, allocate the ip
// to the pod
func (p *FloatingIPPlugin) syncPodIP(pod *corev1.Pod) error {
	if pod.Status.Phase != corev1.PodRunning {
		return nil
	}
	if pod.Annotations == nil {
		return nil
	}
	keyObj, err := util.FormatKey(pod)
	if err != nil {
		glog.V(5).Infof("sync pod %s/%s ip formatKey with error %v", pod.Namespace, pod.Name, err)
		return nil
	}
	ipInfos, err := constant.ParseIPInfo(pod.Annotations[constant.ExtendedCNIArgsAnnotation])
	if err != nil {
		return err
	}
	if len(ipInfos) == 0 || ipInfos[0].IP == nil {
		// should not happen
		return fmt.Errorf("empty ipinfo for pod %s", keyObj.KeyInDB)
	}
	return p.syncIP(keyObj.KeyInDB, ipInfos[0].IP.IP, pod)
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
		if err := p.ipam.AllocateSpecificIP(key, ip, parseReleasePolicy(&pod.ObjectMeta),
			getAttr(pod.Spec.NodeName, string(pod.UID))); err != nil {
			return err
		}
		glog.Infof("updated floatingip %s to key %s", ip.String(), key)
	}
	return nil
}
