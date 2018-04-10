package policy

import (
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
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
		p.syncPodIPInIPSet(newPod, true)
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
		p.syncPodIPInIPSet(pod, false)
	}
	return nil
}

func (p *PolicyManager) AddPolicy(policy *networkv1.NetworkPolicy) error {
	// if a policy is added, we should add policy chain before adding pod rules targeting this chain
	p.syncNetworkPolices()
	p.syncNetworkPolicyRules()
	p.syncPods()
	return nil
}

func (p *PolicyManager) UpdatePolicy(oldPolicy, newPolicy *networkv1.NetworkPolicy) error {
	p.syncNetworkPolices()
	p.syncNetworkPolicyRules()
	return nil
}

func (p *PolicyManager) DeletePolicy(policy *networkv1.NetworkPolicy) error {
	// if a policy is deleted, we should first delete pod rules targeting this policy chain
	p.syncNetworkPolices()
	p.syncPods()
	p.syncNetworkPolicyRules()
	return nil
}
