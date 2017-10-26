/*
Copyright 2017 The Kubernetes Authors.

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
	api "k8s.io/client-go/1.4/pkg/api"
	v1alpha1 "k8s.io/client-go/1.4/pkg/apis/gaia/v1alpha1"
	watch "k8s.io/client-go/1.4/pkg/watch"
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
	UpdateStatus(*v1alpha1.TApp) (*v1alpha1.TApp, error)
	Delete(name string, options *api.DeleteOptions) error
	DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error
	Get(name string) (*v1alpha1.TApp, error)
	List(opts api.ListOptions) (*v1alpha1.TAppList, error)
	Watch(opts api.ListOptions) (watch.Interface, error)
	Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1alpha1.TApp, err error)
	TAppExpansion
}

// tApps implements TAppInterface
type tApps struct {
	client *GaiaClient
	ns     string
}

// newTApps returns a TApps
func newTApps(c *GaiaClient, namespace string) *tApps {
	return &tApps{
		client: c,
		ns:     namespace,
	}
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

func (c *tApps) UpdateStatus(tApp *v1alpha1.TApp) (result *v1alpha1.TApp, err error) {
	result = &v1alpha1.TApp{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("tapps").
		Name(tApp.Name).
		SubResource("status").
		Body(tApp).
		Do().
		Into(result)
	return
}

// Delete takes name of the tApp and deletes it. Returns an error if one occurs.
func (c *tApps) Delete(name string, options *api.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("tapps").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *tApps) DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("tapps").
		VersionedParams(&listOptions, api.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Get takes name of the tApp, and returns the corresponding tApp object, and an error if there is any.
func (c *tApps) Get(name string) (result *v1alpha1.TApp, err error) {
	result = &v1alpha1.TApp{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("tapps").
		Name(name).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of TApps that match those selectors.
func (c *tApps) List(opts api.ListOptions) (result *v1alpha1.TAppList, err error) {
	result = &v1alpha1.TAppList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("tapps").
		VersionedParams(&opts, api.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested tApps.
func (c *tApps) Watch(opts api.ListOptions) (watch.Interface, error) {
	return c.client.Get().
		Prefix("watch").
		Namespace(c.ns).
		Resource("tapps").
		VersionedParams(&opts, api.ParameterCodec).
		Watch()
}

// Patch applies the patch and returns the patched tApp.
func (c *tApps) Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1alpha1.TApp, err error) {
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
