package schedulerplugin

import (
	"git.code.oa.com/gaia/tapp-controller/pkg/client/listers/tappcontroller/v1alpha1"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type PluginFactoryArgs struct {
	Client        *kubernetes.Clientset
	PodLister     corev1lister.PodLister
	TAppLister    v1alpha1.TAppLister
	PodHasSynced  func() bool
	TAppHasSynced func() bool
}
