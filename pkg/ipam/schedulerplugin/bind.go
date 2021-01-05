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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/metrics"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	"tkestack.io/galaxy/pkg/utils/nets"
)

// Bind binds a new floatingip or reuse an old one to pod
func (p *FloatingIPPlugin) Bind(args *schedulerapi.ExtenderBindingArgs) error {
	start := time.Now()
	pod, err := p.PodLister.Pods(args.PodNamespace).Get(args.PodName)
	if err != nil {
		return fmt.Errorf("failed to find pod %s: %w", util.Join(args.PodName, args.PodNamespace), err)
	}
	if !p.hasResourceName(&pod.Spec) {
		// we will config extender resources which ensures pod which doesn't want floatingip won't be sent to plugin
		// see https://github.com/kubernetes/kubernetes/pull/60332
		return fmt.Errorf("pod which doesn't want floatingip have been sent to plugin")
	}
	defer p.lockPod(pod.Name, pod.Namespace)()
	keyObj, err := util.FormatKey(pod)
	if err != nil {
		return err
	}
	cniArgs, err := p.allocateIP(keyObj.KeyInDB, args.Node, pod)
	if err != nil {
		return err
	}
	data, err := json.Marshal(cniArgs)
	if err != nil {
		return fmt.Errorf("marshal cni args %v: %v", *cniArgs, err)
	}
	bindAnnotation := map[string]string{constant.ExtendedCNIArgsAnnotation: string(data)}
	var err1 error
	if err := wait.PollImmediate(time.Millisecond*500, 3*time.Second, func() (bool, error) {
		// It's the extender's response to bind pods to nodes since it is a binder
		if err := p.Client.CoreV1().Pods(args.PodNamespace).Bind(&corev1.Binding{
			ObjectMeta: v1.ObjectMeta{Namespace: args.PodNamespace, Name: args.PodName, UID: args.PodUID,
				Annotations: bindAnnotation},
			Target: corev1.ObjectReference{
				Kind: "Node",
				Name: args.Node,
			},
		}); err != nil {
			err1 = err
			if apierrors.IsNotFound(err) {
				// break retry if pod no longer exists
				return false, err
			}
			return false, nil
		}
		glog.Infof("bind pod %s to %s with %s", keyObj.KeyInDB, args.Node, string(data))
		return true, nil
	}); err != nil {
		if apierrors.IsNotFound(err1) {
			glog.Infof("binding returns not found for pod %s, putting it into unreleased chan", keyObj.KeyInDB)
			// attach ip annotation
			p.unreleased <- &releaseEvent{pod: pod}
		}
		// If fails to update, depending on resync to update
		return fmt.Errorf("update pod %s: %w", keyObj.KeyInDB, err1)
	}
	metrics.ScheduleLatency.WithLabelValues("bind").Observe(time.Since(start).Seconds())
	return nil
}

func (p *FloatingIPPlugin) allocateIP(key string, nodeName string, pod *corev1.Pod) (*constant.CniArgs, error) {
	cniArgs, err := getPodCniArgs(pod)
	if err != nil {
		return nil, err
	}
	ipranges := cniArgs.RequestIPRange
	ipInfos, err := p.ipam.ByKeyAndIPRanges(key, ipranges)
	if err != nil {
		return nil, fmt.Errorf("failed to query floating ip by key %s: %v", key, err)
	}
	if len(ipranges) == 0 && len(ipInfos) > 0 {
		// reuse only one if requesting only one ip
		ipInfos = ipInfos[:1]
	}
	var unallocatedIPRange [][]nets.IPRange // those does not have allocated ips
	reservedIPs := sets.NewString()
	for i := range ipInfos {
		if ipInfos[i] == nil {
			unallocatedIPRange = append(unallocatedIPRange, ipranges[i])
		} else {
			reservedIPs.Insert(ipInfos[i].IP.String())
		}
	}
	policy := parseReleasePolicy(&pod.ObjectMeta)
	attr := floatingip.Attr{Policy: policy, NodeName: nodeName, Uid: string(pod.UID)}
	for _, ipInfo := range ipInfos {
		// check if uid missmatch, if we delete a statfulset/tapp and creates a same name statfulset/tapp immediately,
		// galaxy-ipam may receive bind event for new pod early than deleting event for old pod
		if ipInfo != nil && ipInfo.PodUid != "" && ipInfo.PodUid != string(pod.GetUID()) {
			return nil, fmt.Errorf("waiting for delete event of %s before reuse this ip", key)
		}
	}
	if len(unallocatedIPRange) > 0 || len(ipInfos) == 0 {
		subnet, err := p.queryNodeSubnet(nodeName)
		if err != nil {
			return nil, err
		}
		if _, err := p.ipam.AllocateInSubnetsAndIPRange(key, subnet, unallocatedIPRange, attr); err != nil {
			return nil, err
		}
		ipInfos, err = p.ipam.ByKeyAndIPRanges(key, ipranges)
		if err != nil {
			return nil, fmt.Errorf("failed to query floating ip by key %s: %v", key, err)
		}
	}
	for _, ipInfo := range ipInfos {
		glog.Infof("AssignIP nodeName %s, ip %s, key %s", nodeName, ipInfo.IPInfo.IP.IP.String(), key)
		if err := p.cloudProviderAssignIP(&rpc.AssignIPRequest{
			NodeName:  nodeName,
			IPAddress: ipInfo.IPInfo.IP.IP.String(),
		}); err != nil {
			// do not rollback allocated ip
			return nil, fmt.Errorf("failed to assign ip %s to %s: %v", ipInfo.IPInfo.IP.IP.String(), key, err)
		}
		if reservedIPs.Has(ipInfo.IP.String()) {
			glog.Infof("%s reused %s, updating attr to %v", key, ipInfo.IPInfo.IP.String(), attr)
			if err := p.ipam.UpdateAttr(key, ipInfo.IPInfo.IP.IP, attr); err != nil {
				return nil, fmt.Errorf("failed to update floating ip release policy: %v", err)
			}
		}
	}
	var allocatedIPs []string
	var ret []constant.IPInfo
	for _, ipInfo := range ipInfos {
		if !reservedIPs.Has(ipInfo.IP.String()) {
			allocatedIPs = append(allocatedIPs, ipInfo.IP.String())
		}
		ret = append(ret, ipInfo.IPInfo)
	}
	glog.Infof("%s reused ips %v, allocated ips %v, attr %v", key, reservedIPs.List(), allocatedIPs, attr)
	cniArgs.Common.IPInfos = ret
	return &cniArgs, nil
}

// unbind release ip from pod
func (p *FloatingIPPlugin) unbind(pod *corev1.Pod) error {
	defer p.lockPod(pod.Name, pod.Namespace)()
	glog.V(3).Infof("handle unbind pod %s", pod.Name)
	keyObj, err := util.FormatKey(pod)
	if err != nil {
		return err
	}
	key := keyObj.KeyInDB
	if p.cloudProvider != nil {
		ipInfos, err := p.ipam.ByKeyAndIPRanges(key, nil)
		if err != nil {
			return fmt.Errorf("query floating ip by key %s: %v", key, err)
		}
		for _, ipInfo := range ipInfos {
			ipStr := ipInfo.IPInfo.IP.IP.String()
			glog.Infof("UnAssignIP nodeName %s, ip %s, key %s", ipInfo.NodeName, ipStr, key)
			if err = p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
				NodeName:  ipInfo.NodeName,
				IPAddress: ipStr,
			}); err != nil {
				return fmt.Errorf("failed to unassign ip %s from %s: %v", ipStr, key, err)
			}
		}
	}
	policy := parseReleasePolicy(&pod.ObjectMeta)
	if keyObj.Deployment() {
		return p.unbindDpPod(keyObj, policy, "during unbinding pod")
	}
	return p.unbindNoneDpPod(keyObj, policy, "during unbinding pod")
}

func (p *FloatingIPPlugin) Release(r *ReleaseRequest) error {
	caller := "by " + getCaller()
	k := r.KeyObj
	defer p.lockPod(k.PodName, k.Namespace)()
	// we are holding the pod's lock, query again in case the ip has been reallocated.
	fip, err := p.ipam.ByIP(r.IP)
	if err != nil {
		return err
	}
	if fip.Key != k.KeyInDB {
		// if key changed, abort
		if fip.Key == "" {
			glog.Infof("attempt to release %s key %s which is already released", r.IP.String(), k.KeyInDB)
			return nil
		}
		return fmt.Errorf("ip allocated to another pod %s", fip.Key)
	}
	running, reason := p.podRunning(k.PodName, k.Namespace, fip.PodUid)
	if running {
		return fmt.Errorf("pod %s_%s (uid %s) is running", k.Namespace, k.PodName, fip.PodUid)
	}
	glog.Infof("%s is not running, %s, %s", k.KeyInDB, reason, caller)
	if p.cloudProvider != nil && fip.NodeName != "" {
		// For tapp and sts pod, nodeName will be updated to empty after unassigning
		glog.Infof("UnAssignIP nodeName %s, ip %s, key %s %s", fip.NodeName, r.IP.String(), k.KeyInDB, caller)
		if err := p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
			NodeName:  fip.NodeName,
			IPAddress: fip.IP.String(),
		}); err != nil {
			return fmt.Errorf("UnAssignIP nodeName %s, ip %s: %v", fip.NodeName, fip.IP.String(), err)
		}
		// for tapp and sts pod, we need to clean its node attr and uid
		if err := p.reserveIP(k.KeyInDB, k.KeyInDB, "after UnAssignIP "+caller); err != nil {
			return err
		}
	}
	if err := p.ipam.Release(k.KeyInDB, r.IP); err != nil {
		glog.Errorf("release ip %s: %v", caller, err)
		return fmt.Errorf("release ip: %v", err)
	}
	glog.Infof("released floating ip %s from %s %s", r.IP.String(), k.KeyInDB, caller)
	return nil
}
