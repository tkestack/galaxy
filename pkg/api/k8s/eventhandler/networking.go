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
package eventhandler

import (
	networkv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/cache"
	glog "k8s.io/klog"
)

type NetworkPolicyWatcher interface {
	AddPolicy(policy *networkv1.NetworkPolicy) error
	UpdatePolicy(oldPolicy, newPolicy *networkv1.NetworkPolicy) error
	DeletePolicy(policy *networkv1.NetworkPolicy) error
}

var (
	_ = cache.ResourceEventHandler(&NetworkPolicyEventHandler{})
)

type NetworkPolicyEventHandler struct {
	watcher NetworkPolicyWatcher
}

func NewNetworkPolicyEventHandler(watcher NetworkPolicyWatcher) *NetworkPolicyEventHandler {
	return &NetworkPolicyEventHandler{watcher: watcher}
}

func (e *NetworkPolicyEventHandler) OnAdd(obj interface{}) {
	policy, ok := obj.(*networkv1.NetworkPolicy)
	if !ok {
		glog.Errorf("cannot convert newObj to *networkv1.NetworkPolicy: %v", obj)
		return
	}
	glog.V(5).Infof("Add policy %s_%s", policy.Name, policy.Namespace)
	if err := e.watcher.AddPolicy(policy); err != nil {
		glog.Errorf("AddPolicy failed: %v", err)
	}
}

func (e *NetworkPolicyEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPolicy, ok := oldObj.(*networkv1.NetworkPolicy)
	if !ok {
		glog.Errorf("cannot convert oldObj to *networkv1.NetworkPolicy: %v", oldObj)
		return
	}
	newPolicy, ok := newObj.(*networkv1.NetworkPolicy)
	if !ok {
		glog.Errorf("cannot convert newObj to *networkv1.NetworkPolicy: %v", newObj)
		return
	}
	glog.V(5).Infof("Update policy %s_%s", newPolicy.Name, newPolicy.Namespace)
	if err := e.watcher.UpdatePolicy(oldPolicy, newPolicy); err != nil {
		glog.Errorf("UpdatePolicy failed: %v", err)
	}
}

func (e *NetworkPolicyEventHandler) OnDelete(obj interface{}) {
	policy, ok := obj.(*networkv1.NetworkPolicy)
	if !ok {
		glog.Errorf("cannot convert newObj to *networkv1.NetworkPolicy: %v", obj)
		return
	}
	glog.V(5).Infof("Delete policy %s_%s", policy.Name, policy.Namespace)
	if err := e.watcher.DeletePolicy(policy); err != nil {
		glog.Errorf("DeletePolicy failed: %v", err)
	}
}
