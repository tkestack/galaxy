package schedulerplugin

import (
	crd_clientset "git.code.oa.com/gaiastack/galaxy/pkg/ipam/client/clientset/versioned"
	list "git.code.oa.com/gaiastack/galaxy/pkg/ipam/client/listers/galaxy/v1alpha1"
	"k8s.io/client-go/kubernetes"
	appv1 "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type PluginFactoryArgs struct {
	Client            kubernetes.Interface
	PodLister         corev1lister.PodLister
	StatefulSetLister appv1.StatefulSetLister
	DeploymentLister  appv1.DeploymentLister
	PoolLister        list.PoolLister
	PodHasSynced      func() bool
	StatefulSetSynced func() bool
	DeploymentSynced  func() bool
	CrdClient         crd_clientset.Interface
}
