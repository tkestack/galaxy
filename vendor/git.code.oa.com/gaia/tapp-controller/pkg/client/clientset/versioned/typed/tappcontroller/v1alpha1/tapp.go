/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	v1alpha1 "git.code.oa.com/gaia/tapp-controller/pkg/apis/tappcontroller/v1alpha1"
	scheme "git.code.oa.com/gaia/tapp-controller/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// TAppsGetter has a method to return a TAppInterface.
// A group's client should implement this interface.
type TAppsGetter interface {
	TApps(namespace string) TAppInterface
}

// TAppInterface has methods to work with TApp resources.
type TAppInterface interface {
	Create(*v1alpha1.TApp) (*v1alpha1.TApp, error)
	Update(*v1alpha1.TApp) (*v1alpha1.TApp, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.TApp, error)
	List(opts v1.ListOptions) (*v1alpha1.TAppList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.TApp, err error)
	TAppExpansion
}

// tApps implements TAppInterface
type tApps struct {
	client rest.Interface
	ns     string
}

// newTApps returns a TApps
func newTApps(c *TappcontrollerV1alpha1Client, namespace string) *tApps {
	return &tApps{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the tApp, and returns the corresponding tApp object, and an error if there is any.
func (c *tApps) Get(name string, options v1.GetOptions) (result *v1alpha1.TApp, err error) {
	result = &v1alpha1.TApp{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("tapps").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of TApps that match those selectors.
func (c *tApps) List(opts v1.ListOptions) (result *v1alpha1.TAppList, err error) {
	result = &v1alpha1.TAppList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("tapps").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested tApps.
func (c *tApps) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("tapps").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a tApp and creates it.  Returns the server's representation of the tApp, and an error, if there is any.
func (c *tApps) Create(tApp *v1alpha1.TApp) (result *v1alpha1.TApp, err error) {
	result = &v1alpha1.TApp{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("tapps").
		Body(tApp).
		Do().
		Into(result)
	return
}

// Update takes the representation of a tApp and updates it. Returns the server's representation of the tApp, and an error, if there is any.
func (c *tApps) Update(tApp *v1alpha1.TApp) (result *v1alpha1.TApp, err error) {
	result = &v1alpha1.TApp{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("tapps").
		Name(tApp.Name).
		Body(tApp).
		Do().
		Into(result)
	return
}

// Delete takes name of the tApp and deletes it. Returns an error if one occurs.
func (c *tApps) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("tapps").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *tApps) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("tapps").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched tApp.
func (c *tApps) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.TApp, err error) {
	result = &v1alpha1.TApp{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("tapps").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
