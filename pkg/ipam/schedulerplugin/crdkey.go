/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package schedulerplugin

import (
	"sync"

	extensionlister "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	"tkestack.io/galaxy/pkg/ipam/crd"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

// CrdKey stores schema.GroupVersionResource for each crd workloads and offers query funcs to get
// schema.GroupVersionResource of a appPrefix
type CrdKey interface {
	// GetGroupVersionResource returns the schema.GroupVersionResource for the stored app key prefix
	GetGroupVersionResource(appPrefix string) *schema.GroupVersionResource
}

// NewCrdKey creates a CrdKey
func NewCrdKey(extensionLister extensionlister.CustomResourceDefinitionLister) CrdKey {
	return &crdKey{
		extensionLister: extensionLister,
		keyToGVR:        make(map[string]*schema.GroupVersionResource),
	}
}

var _ CrdKey = &crdKey{}

// crdKey is the implementable of CrdKey
type crdKey struct {
	extensionLister extensionlister.CustomResourceDefinitionLister
	sync.Mutex
	keyToGVR map[string]*schema.GroupVersionResource
}

func (c *crdKey) GetGroupVersionResource(appPrefix string) *schema.GroupVersionResource {
	c.Lock()
	defer c.Unlock()
	if _, ok := c.keyToGVR[appPrefix]; !ok {
		if err := c.popularCache(); err != nil {
			klog.Warning(err)
		}
	}
	return c.keyToGVR[appPrefix]
}

func (c *crdKey) popularCache() error {
	crds, err := c.extensionLister.List(labels.Everything())
	if err != nil {
		return err
	}
	for i := range crds {
		subRes := crds[i].Spec.Subresources
		if subRes == nil || subRes.Scale == nil || subRes.Scale.SpecReplicasPath == "" {
			continue
		}
		// TODO kind conflict ?
		appTypePrefix := util.GetAppTypePrefix(crds[i].Spec.Names.Kind)
		gvr := crd.GetGroupVersionResource(crds[i])
		c.keyToGVR[appTypePrefix] = &gvr
	}
	return nil
}
