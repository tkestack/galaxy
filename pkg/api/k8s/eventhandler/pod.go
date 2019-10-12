package eventhandler

import (
	glog "k8s.io/klog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
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
