package schedulerplugin

import (
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/cache"
	"k8s.io/client-go/1.4/kubernetes"
)

type PluginFactoryArgs struct {
	PodLister     *cache.StoreToPodLister
	TAppLister    *cache.StoreToTAppLister
	Client        *kubernetes.Clientset
	PodHasSynced  func() bool
	TAppHasSynced func() bool
}
