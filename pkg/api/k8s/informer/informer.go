package informer

import (
	galaxycache "git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/cache"
	"k8s.io/client-go/1.4/pkg/api/v1"
	extensionsv1 "k8s.io/client-go/1.4/pkg/apis/extensions/v1beta1"
	gaiav1 "k8s.io/client-go/1.4/pkg/apis/gaia/v1alpha1"
	"k8s.io/client-go/1.4/tools/cache"

	"github.com/golang/glog"
)

type PodInformer struct {
	// a means to list all pods.
	PodLister *galaxycache.StoreToPodLister

	PodPopulator *cache.Controller

	// A map of pod watchers
	PodWatchers map[string]PodWatcher
}

type ReplicaSetInformer struct {
	// a means to list all replicasets
	ReplicaSetLister *galaxycache.StoreToReplicaSetLister

	ReplicaSetPopulator *cache.Controller

	// A map of replicaSet watchers
	ReplicaSetWatchers map[string]ReplicaSetWatcher
}

type TAppInformer struct {
	// a means to list all TApps
	TAppLister *galaxycache.StoreToTAppLister

	TAppPopulator *cache.Controller

	// A map of TApp watchers
	TAppWatchers map[string]TAppWatcher
}

type PodWatcher interface {
	AddPod(pod *v1.Pod) error

	UpdatePod(oldPod, newPod *v1.Pod) error

	RemovePod(pod *v1.Pod) error
}

type ReplicaSetWatcher interface {
	AddReplicaSet(replicaSet *extensionsv1.ReplicaSet) error

	UpdateReplicaSet(oldReplicaSet, newReplicaSet *extensionsv1.ReplicaSet) error

	RemoveReplicaSet(replicaSet *extensionsv1.ReplicaSet) error
}

type TAppWatcher interface {
	AddTApp(tapp *gaiav1.TApp) error

	UpdateTApp(oldTApp, newTApp *gaiav1.TApp) error

	RemoveTApp(tApp *gaiav1.TApp) error
}

func (c *PodInformer) AddPodToCache(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		glog.Errorf("cannot convert to *v1.Pod: %v", obj)
		return
	}
	for name, watcher := range c.PodWatchers {
		if err := watcher.AddPod(pod); err != nil {
			glog.Errorf("%s AddPod failed: %v", name, err)
		}
	}
}

func (c *PodInformer) UpdatePodInCache(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*v1.Pod)
	if !ok {
		glog.Errorf("cannot convert oldObj to *v1.Pod: %v", oldObj)
		return
	}
	newPod, ok := newObj.(*v1.Pod)
	if !ok {
		glog.Errorf("cannot convert newObj to *v1.Pod: %v", newObj)
		return
	}
	for name, watcher := range c.PodWatchers {
		if err := watcher.UpdatePod(oldPod, newPod); err != nil {
			glog.Errorf("%s UpdatePod failed: %v", name, err)
		}
	}
}

func (c *PodInformer) DeletePodFromCache(obj interface{}) {
	var pod *v1.Pod
	switch t := obj.(type) {
	case *v1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		pod, ok = t.Obj.(*v1.Pod)
		if !ok {
			glog.Errorf("cannot convert to *v1.Pod: %v", t.Obj)
			return
		}
	default:
		glog.Errorf("cannot convert to *v1.Pod: %v", t)
		return
	}
	for name, watcher := range c.PodWatchers {
		if err := watcher.RemovePod(pod); err != nil {
			glog.Errorf("%s RemovePod failed: %v", name, err)
		}
	}
}

func (c *ReplicaSetInformer) AddReplicaSet(obj interface{}) {
	replicaSet, ok := obj.(*extensionsv1.ReplicaSet)
	if !ok {
		glog.Errorf("cannot convert to *extensionsv1.ReplicaSet: %v", obj)
		return
	}
	for name, watcher := range c.ReplicaSetWatchers {
		if err := watcher.AddReplicaSet(replicaSet); err != nil {
			glog.Errorf("%s AddReplicaSet failed: %v", name, err)
		}
	}
}

func (c *ReplicaSetInformer) UpdateReplicaSet(oldObj, newObj interface{}) {
	oldReplicaSet, ok := oldObj.(*extensionsv1.ReplicaSet)
	if !ok {
		glog.Errorf("cannot convert oldObj to *extensionsv1.ReplicaSet: %v", oldObj)
		return
	}
	newReplicaSet, ok := newObj.(*extensionsv1.ReplicaSet)
	if !ok {
		glog.Errorf("cannot convert newObj to *extensionsv1.ReplicaSet: %v", newObj)
		return
	}
	for name, watcher := range c.ReplicaSetWatchers {
		if err := watcher.UpdateReplicaSet(oldReplicaSet, newReplicaSet); err != nil {
			glog.Errorf("%s UpdateReplicaSet failed: %v", name, err)
		}
	}
}

func (c *ReplicaSetInformer) DeleteReplicaSet(obj interface{}) {
	var replicaSet *extensionsv1.ReplicaSet
	switch t := obj.(type) {
	case *extensionsv1.ReplicaSet:
		replicaSet = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		replicaSet, ok = t.Obj.(*extensionsv1.ReplicaSet)
		if !ok {
			glog.Errorf("cannot convert to *extensionsv1.ReplicaSet: %v", t.Obj)
			return
		}
	default:
		glog.Errorf("cannot convert to *extensionsv1.ReplicaSet: %v", t)
		return
	}
	for name, watcher := range c.ReplicaSetWatchers {
		if err := watcher.RemoveReplicaSet(replicaSet); err != nil {
			glog.Errorf("%s RemoveReplicaSet failed: %v", name, err)
		}
	}
}

func (c *TAppInformer) AddTApp(obj interface{}) {
	tApp, ok := obj.(*gaiav1.TApp)
	if !ok {
		glog.Errorf("cannot convert to *gaiav1.TApp: %v", obj)
		return
	}
	for name, watcher := range c.TAppWatchers {
		if err := watcher.AddTApp(tApp); err != nil {
			glog.Errorf("%s AddTApp failed: %v", name, err)
		}
	}
}

func (c *TAppInformer) UpdateTApp(oldObj, newObj interface{}) {
	oldTApp, ok := oldObj.(*gaiav1.TApp)
	if !ok {
		glog.Errorf("cannot convert oldObj to *gaiav1.TApp: %v", oldObj)
		return
	}
	newTApp, ok := newObj.(*gaiav1.TApp)
	if !ok {
		glog.Errorf("cannot convert newObj to *gaiav1.TApp: %v", newObj)
		return
	}
	for name, watcher := range c.TAppWatchers {
		if err := watcher.UpdateTApp(oldTApp, newTApp); err != nil {
			glog.Errorf("%s UpdateTApp failed: %v", name, err)
		}
	}
}

func (c *TAppInformer) DeleteTApp(obj interface{}) {
	var tApp *gaiav1.TApp
	switch t := obj.(type) {
	case *gaiav1.TApp:
		tApp = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		tApp, ok = t.Obj.(*gaiav1.TApp)
		if !ok {
			glog.Errorf("cannot convert to *gaiav1.TApp: %v", t.Obj)
			return
		}
	default:
		glog.Errorf("cannot convert to *gaiav1.TApp: %v", t)
		return
	}
	for name, watcher := range c.TAppWatchers {
		if err := watcher.RemoveTApp(tApp); err != nil {
			glog.Errorf("%s RemoveTApp failed: %v", name, err)
		}
	}
}
