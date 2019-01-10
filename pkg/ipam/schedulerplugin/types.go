package schedulerplugin

import (
	"git.code.oa.com/gaia/tapp-controller/pkg/client/clientset/versioned"
	"git.code.oa.com/gaia/tapp-controller/pkg/client/listers/tappcontroller/v1alpha1"
	"k8s.io/client-go/kubernetes"
	appv1 "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type PluginFactoryArgs struct {
	Client            *kubernetes.Clientset
	TAppClient        *versioned.Clientset
	PodLister         corev1lister.PodLister
	TAppLister        v1alpha1.TAppLister
	StatefulSetLister appv1.StatefulSetLister
	PodHasSynced      func() bool
	TAppHasSynced     func() bool
	StatefulSetSynced func() bool
}
