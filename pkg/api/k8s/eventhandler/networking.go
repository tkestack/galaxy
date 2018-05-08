package eventhandler

import (
	"github.com/golang/glog"
	networkv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/cache"
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
