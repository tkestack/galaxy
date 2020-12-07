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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/metrics"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	"tkestack.io/galaxy/pkg/utils/nets"
)

// Filter marks nodes which have no available ips as FailedNodes
// If the given pod doesn't want floating IP, none failedNodes returns
func (p *FloatingIPPlugin) Filter(pod *corev1.Pod, nodes []corev1.Node) ([]corev1.Node, schedulerapi.FailedNodesMap,
	error) {
	start := time.Now()
	failedNodesMap := schedulerapi.FailedNodesMap{}
	if !p.hasResourceName(&pod.Spec) {
		return nodes, failedNodesMap, nil
	}
	filteredNodes := []corev1.Node{}
	defer p.lockPod(pod.Name, pod.Namespace)()
	subnetSet, err := p.getSubnet(pod)
	if err != nil {
		return filteredNodes, failedNodesMap, err
	}
	for i := range nodes {
		nodeName := nodes[i].Name
		subnet, err := p.getNodeSubnet(&nodes[i])
		if err != nil {
			failedNodesMap[nodes[i].Name] = err.Error()
			continue
		}
		if subnetSet.Has(subnet.String()) {
			filteredNodes = append(filteredNodes, nodes[i])
		} else {
			failedNodesMap[nodeName] = "FloatingIPPlugin:NoFIPLeft"
		}
	}
	if glog.V(5) {
		nodeNames := make([]string, len(filteredNodes))
		for i := range filteredNodes {
			nodeNames[i] = filteredNodes[i].Name
		}
		glog.V(5).Infof("filtered nodes %v failed nodes %v for %s_%s", nodeNames, failedNodesMap,
			pod.Namespace, pod.Name)
	}
	metrics.ScheduleLatency.WithLabelValues("filter").Observe(time.Since(start).Seconds())
	return filteredNodes, failedNodesMap, nil
}

// #lizard forgives
func (p *FloatingIPPlugin) getSubnet(pod *corev1.Pod) (sets.String, error) {
	keyObj, err := util.FormatKey(pod)
	if err != nil {
		return nil, err
	}
	cniArgs, err := getPodCniArgs(pod)
	if err != nil {
		return nil, err
	}
	ipranges := cniArgs.RequestIPRange
	// first check if exists an already allocated ip for this pod
	ipInfos, err := p.ipam.ByKeyAndIPRanges(keyObj.KeyInDB, ipranges)
	if err != nil {
		return nil, fmt.Errorf("failed to query by key %s: %v", keyObj.KeyInDB, err)
	}
	allocatedSubnets := sets.NewString()
	if len(ipranges) == 0 {
		if len(ipInfos) > 0 {
			glog.V(3).Infof("%s already have an allocated ip %s in subnets %v", keyObj.KeyInDB,
				ipInfos[0].IP.String(), ipInfos[0].NodeSubnets)
			return ipInfos[0].NodeSubnets, nil
		}
	} else {
		var unallocatedIPRange [][]nets.IPRange // those does not have allocated ips
		var ips []string
		for i := range ipInfos {
			if ipInfos[i] == nil {
				unallocatedIPRange = append(unallocatedIPRange, ipranges[i])
			} else {
				ips = append(ips, ipInfos[i].IP.String())
				if allocatedSubnets.Len() == 0 {
					allocatedSubnets.Insert(ipInfos[i].NodeSubnets.UnsortedList()...)
				} else {
					allocatedSubnets = allocatedSubnets.Intersection(ipInfos[i].NodeSubnets)
				}
			}
		}
		if len(unallocatedIPRange) == 0 {
			glog.V(3).Infof("%s already have allocated ips %v with intersection subnets %v",
				keyObj.KeyInDB, ips, allocatedSubnets)
			return allocatedSubnets, nil
		}
		glog.V(3).Infof("%s have allocated ips %v with intersection subnets %v, but also unallocated "+
			"ip ranges %v", keyObj.KeyInDB, ips, allocatedSubnets, unallocatedIPRange)
		ipranges = unallocatedIPRange
	}
	policy := parseReleasePolicy(&pod.ObjectMeta)
	if policy != constant.ReleasePolicyPodDelete {
		if err1 := p.supportReserveIPPolicy(keyObj, policy); err1 != nil {
			return nil, fmt.Errorf("release policy %s is not supported for pod %s: %w",
				constant.PolicyStr(policy), keyObj.PodName, err1)
		}
	}
	var replicas int
	var isPoolSizeDefined bool
	if keyObj.Deployment() {
		replicas, isPoolSizeDefined, err = p.getDpReplicas(keyObj)
		if err != nil {
			return nil, err
		}
		// Lock to make checking available subnets and allocating reserved ip atomic
		defer p.LockDpPool(keyObj.PoolPrefix())()
	}
	subnetSet, reserve, err := p.getAvailableSubnet(keyObj, policy, replicas, isPoolSizeDefined, ipranges)
	if err != nil {
		return nil, err
	}
	if allocatedSubnets.Len() > 0 {
		subnetSet = subnetSet.Intersection(allocatedSubnets)
	}
	if (reserve || isPoolSizeDefined) && subnetSet.Len() > 0 {
		// Since bind is in a different goroutine than filter in scheduler, we can't ensure this pod got binded
		// before the next one got filtered to ensure max size of allocated ips.
		// So we'd better do the allocate in filter for reserve situation.
		reserveSubnet := subnetSet.List()[0]
		subnetSet = sets.NewString(reserveSubnet)
		if err := p.allocateDuringFilter(keyObj, reserve, isPoolSizeDefined, reserveSubnet, policy,
			string(pod.UID)); err != nil {
			return nil, err
		}
	}
	return subnetSet, nil
}

func (p *FloatingIPPlugin) allocateDuringFilter(keyObj *util.KeyObj, reserve, isPoolSizeDefined bool,
	reserveSubnet string, policy constant.ReleasePolicy, uid string) error {
	// we can't get nodename during filter, update attr on bind
	attr := floatingip.Attr{Policy: policy, NodeName: "", Uid: uid}
	if reserve {
		if err := p.allocateInSubnetWithKey(keyObj.PoolPrefix(), keyObj.KeyInDB, reserveSubnet, attr,
			"filter"); err != nil {
			return err
		}
	} else if isPoolSizeDefined {
		// if pool size defined and we got no reserved IP, we need to allocate IP from empty key
		_, ipNet, err := net.ParseCIDR(reserveSubnet)
		if err != nil {
			return err
		}
		if err := p.allocateInSubnet(keyObj.KeyInDB, ipNet, attr, "filter"); err != nil {
			return err
		}
	}
	return nil
}
