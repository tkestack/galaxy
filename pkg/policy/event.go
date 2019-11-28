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
package policy

import (
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	glog "k8s.io/klog"
)

func (p *PolicyManager) AddPod(pod *corev1.Pod) error {
	return nil
}

func (p *PolicyManager) UpdatePod(oldPod, newPod *corev1.Pod) error {
	if newPod.Spec.NodeName == p.hostName {
		if err := p.SyncPodChains(newPod); err != nil {
			glog.Warning(err)
		}
	}
	if newPod.Status.PodIP != "" {
		p.SyncPodIPInIPSet(newPod, true)
	}
	return nil
}

func (p *PolicyManager) DeletePod(pod *corev1.Pod) error {
	if pod.Spec.NodeName == p.hostName {
		if err := p.deletePodChains(pod); err != nil {
			glog.Warning(err)
		}
	}
	if pod.Status.PodIP != "" {
		p.SyncPodIPInIPSet(pod, false)
	}
	return nil
}

func (p *PolicyManager) AddPolicy(policy *networkv1.NetworkPolicy) error {
	p.startPodInformerFactory()
	// if a policy is added, we should add policy chain before adding pod rules targeting this chain
	p.syncNetworkPolices()
	p.syncNetworkPolicyRules()
	p.syncPods()
	return nil
}

func (p *PolicyManager) UpdatePolicy(oldPolicy, newPolicy *networkv1.NetworkPolicy) error {
	p.syncNetworkPolices()
	p.syncNetworkPolicyRules()
	p.syncPods()
	return nil
}

func (p *PolicyManager) DeletePolicy(policy *networkv1.NetworkPolicy) error {
	// if a policy is deleted, we should first delete pod rules targeting this policy chain
	p.syncNetworkPolices()
	p.syncPods()
	p.syncNetworkPolicyRules()
	return nil
}
