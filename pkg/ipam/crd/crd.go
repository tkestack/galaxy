package crd

import (
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/ipam/apis/galaxy"
)

// floatingipCrd is the crd format of floatingip
var floatingipCrd = &extensionsv1.CustomResourceDefinition{
	ObjectMeta: metav1.ObjectMeta{
		Name: "floatingips.galaxy.k8s.io",
	},
	TypeMeta: metav1.TypeMeta{
		Kind:       "CustomResourceDefinition",
		APIVersion: "apiextensions.k8s.io/v1beta1",
	},
	Spec: extensionsv1.CustomResourceDefinitionSpec{
		Group:   galaxy.GroupName,
		Version: "v1alpha1",
		Scope:   extensionsv1.ClusterScoped,
		Names: extensionsv1.CustomResourceDefinitionNames{
			Kind:       "FloatingIP",
			Plural:     "floatingips",
			ShortNames: []string{"fip"},
		},
	},
}

// poolCrd is the crd format of pool
var poolCrd = &extensionsv1.CustomResourceDefinition{
	ObjectMeta: metav1.ObjectMeta{
		Name: "pools.galaxy.k8s.io",
	},
	TypeMeta: metav1.TypeMeta{
		Kind:       "CustomResourceDefinition",
		APIVersion: "apiextensions.k8s.io/v1beta1",
	},
	Spec: extensionsv1.CustomResourceDefinitionSpec{
		Group:   galaxy.GroupName,
		Version: "v1alpha1",
		Scope:   extensionsv1.NamespaceScoped,
		Names: extensionsv1.CustomResourceDefinitionNames{
			Kind:   "Pool",
			Plural: "pools",
		},
	},
}

// EnsureCRDCreated ensures floatingip and pool are created in apiserver
func EnsureCRDCreated(client apiextensionsclient.Interface) error {
	crdClient := client.ApiextensionsV1beta1().CustomResourceDefinitions()
	crds := []*extensionsv1.CustomResourceDefinition{floatingipCrd, poolCrd}
	for i := range crds {
		// try to create each crd and ignores already exist error
		if _, err := crdClient.Create(crds[i]); err != nil && !apierrors.IsAlreadyExists(err) {
			glog.Errorf("Error creating CRD: %s", crds[i].Spec.Names.Kind)
			return err
		}
		glog.Infof("Create CRD %s successfully.", crds[i].Spec.Names.Kind)
	}
	return nil
}
