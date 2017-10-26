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
	registered "k8s.io/client-go/1.4/pkg/apimachinery/registered"
	serializer "k8s.io/client-go/1.4/pkg/runtime/serializer"
	rest "k8s.io/client-go/1.4/rest"
)

type GaiaInterface interface {
	GetRESTClient() *rest.RESTClient
	TAppsGetter
}

// GaiaClient is used to interact with features provided by the Gaia group.
type GaiaClient struct {
	*rest.RESTClient
}

func (c *GaiaClient) TApps(namespace string) TAppInterface {
	return newTApps(c, namespace)
}

// NewForConfig creates a new GaiaClient for the given config.
func NewForConfig(c *rest.Config) (*GaiaClient, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &GaiaClient{client}, nil
}

// NewForConfigOrDie creates a new GaiaClient for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *GaiaClient {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new GaiaClient for the given RESTClient.
func New(c *rest.RESTClient) *GaiaClient {
	return &GaiaClient{c}
}

func setConfigDefaults(config *rest.Config) error {
	// if gaia group is not registered, return an error
	g, err := registered.Group("gaia")
	if err != nil {
		return err
	}
	config.APIPath = "/apis"
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	// TODO: Unconditionally set the config.Version, until we fix the config.
	//if config.Version == "" {
	copyGroupVersion := g.GroupVersion
	config.GroupVersion = &copyGroupVersion
	//}

	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}

	return nil
}

// GetRESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *GaiaClient) GetRESTClient() *rest.RESTClient {
	if c == nil {
		return nil
	}
	return c.RESTClient
}
