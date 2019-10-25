package schedulerplugin

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

// releaseEvent keeps track of retried times of unbinding pod
type releaseEvent struct {
	pod        *corev1.Pod
	retryTimes int
}

// AddPod does nothing
func (p *FloatingIPPlugin) AddPod(pod *corev1.Pod) error {
	return nil
}

// UpdatePod syncs pod ip with ipam
func (p *FloatingIPPlugin) UpdatePod(oldPod, newPod *corev1.Pod) error {
	if !p.hasResourceName(&newPod.Spec) {
		return nil
	}
	if !evicted(oldPod) && evicted(newPod) {
		// Deployments will leave evicted pods
		// If it's a evicted one, release its ip
		p.unreleased <- &releaseEvent{pod: newPod}
	}
	if err := p.syncPodIP(newPod); err != nil {
		glog.Warningf("failed to sync pod ip: %v", err)
	}
	return nil
}

// DeletePod unbinds pod from ipam
func (p *FloatingIPPlugin) DeletePod(pod *corev1.Pod) error {
	if !p.hasResourceName(&pod.Spec) {
		return nil
	}
	glog.Infof("handle pod delete event: %s_%s", pod.Name, pod.Namespace)
	p.unreleased <- &releaseEvent{pod: pod}
	return nil
}

// loop pulls release event from chan and calls unbind to unbind pod
func (p *FloatingIPPlugin) loop(stop chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case event := <-p.unreleased:
			if err := p.unbind(event.pod); err != nil {
				event.retryTimes++
				glog.Warningf("unbind pod %s failed for %d times: %v", util.PodName(event.pod), event.retryTimes, err)
				if event.retryTimes > 3 {
					// leave it to resync to protect chan from explosion
					glog.Errorf("abort unbind for pod %s, retried %d times: %v", util.PodName(event.pod), event.retryTimes, err)
				} else {
					go func() {
						// backoff time if required
						time.Sleep(300 * time.Millisecond * time.Duration(event.retryTimes))
						p.unreleased <- event
					}()
				}
			}
		}
	}
}
