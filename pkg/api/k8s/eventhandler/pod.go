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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	glog "k8s.io/klog"
)

type PodWatcher interface {
	AddPod(pod *corev1.Pod) error
	UpdatePod(oldPod, newPod *corev1.Pod) error
	DeletePod(pod *corev1.Pod) error
}

var (
	_ = cache.ResourceEventHandler(&PodEventHandler{})
)

type PodEventHandler struct {
	watcher PodWatcher
}

func NewPodEventHandler(watcher PodWatcher) *PodEventHandler {
	return &PodEventHandler{watcher: watcher}
}

func (e *PodEventHandler) OnAdd(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		glog.Errorf("cannot convert newObj to *corev1.Pod: %v", obj)
		return
	}
	glog.V(5).Infof("Add pod %s_%s", pod.Name, pod.Namespace)
	if err := e.watcher.AddPod(pod); err != nil {
		glog.Errorf("AddPod failed: %v", err)
	}
}

func (e *PodEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		glog.Errorf("cannot convert oldObj to *corev1.Pod: %v", oldObj)
		return
	}
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		glog.Errorf("cannot convert newObj to *corev1.Pod: %v", newObj)
		return
	}
	glog.V(5).Infof("Update pod %s_%s", newPod.Name, newPod.Namespace)
	if err := e.watcher.UpdatePod(oldPod, newPod); err != nil {
		glog.Errorf("UpdatePod failed: %v", err)
	}
}

func (e *PodEventHandler) OnDelete(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		glog.Errorf("cannot convert newObj to *corev1.Pod: %v", obj)
		return
	}
	glog.V(5).Infof("Delete pod %s_%s", pod.Name, pod.Namespace)
	if err := e.watcher.DeletePod(pod); err != nil {
		glog.Errorf("RemovePod failed: %v", err)
	}
}
