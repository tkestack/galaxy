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
	if !finished(oldPod) && finished(newPod) {
		// Deployments will leave evicted pods
		// If it's a evicted one, release its ip
		glog.Infof("release ip from %s_%s, phase %s", newPod.Name, newPod.Namespace, string(newPod.Status.Phase))
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
			go func(event *releaseEvent) {
				if err := p.unbind(event.pod); err != nil {
					event.retryTimes++
					if event.retryTimes > 3 {
						// leave it to resync to protect chan from explosion
						glog.Errorf("abort unbind for pod %s, retried %d times: %v", util.PodName(event.pod),
							event.retryTimes, err)
					} else {
						glog.Warningf("unbind pod %s failed for %d times: %v", util.PodName(event.pod),
							event.retryTimes, err)
						// backoff time if required
						time.Sleep(100 * time.Millisecond * time.Duration(event.retryTimes))
						p.unreleased <- event
					}
				}
			}(event)
		}
	}
}
