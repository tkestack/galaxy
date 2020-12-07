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
package crd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	extensionlister "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

// CrdCache caches CustomResourceDefinition objects via dynamic informers to make searching for VPA controller and target selectors fast.
type CrdCache interface {
	// GetReplicas returns the given crd object's replicas
	GetReplicas(gvr schema.GroupVersionResource, namespace, name string) (int, error)
}

// NewCrdCache returns new instance of CrdCache
func NewCrdCache(dynamicClient dynamic.Interface, extensionLister extensionlister.CustomResourceDefinitionLister,
	resyncTime time.Duration) CrdCache {
	return &crdCache{
		dynamicFactory:   dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, resyncTime),
		startedInformers: map[schema.GroupVersionResource]bool{},
		extensionLister:  extensionLister,
	}
}

type crdCache struct {
	dynamicFactory  dynamicinformer.DynamicSharedInformerFactory
	extensionLister extensionlister.CustomResourceDefinitionLister
	lock            sync.Mutex
	// startedInformers caches started GroupVersionResource informers
	startedInformers map[schema.GroupVersionResource]bool
}

func (c *crdCache) GetReplicas(gvr schema.GroupVersionResource, namespace, name string) (int, error) {
	crd, err := c.extensionLister.Get(gvr.GroupResource().String())
	if err != nil {
		return 0, err
	}
	subRes := crd.Spec.Subresources
	if subRes == nil || subRes.Scale == nil {
		return 0, nil
	}
	lister := c.getLister(gvr)
	obj, err := lister.ByNamespace(namespace).Get(name)
	if err != nil {
		return 0, err
	}
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return 0, fmt.Errorf("crd object %s %s/%s can't be converted to a unstructured object", gvr.String(), name, namespace)
	}
	return replicasOfCustomResource(unstructuredObj, subRes.Scale.SpecReplicasPath)
}

// getLister gets the lister for the given GroupVersionResource and makes sure the informer have started and synced
func (c *crdCache) getLister(gvr schema.GroupVersionResource) cache.GenericLister {
	informer := c.dynamicFactory.ForResource(gvr)
	c.lock.Lock()
	defer c.lock.Unlock()
	if _, ok := c.startedInformers[gvr]; !ok {
		klog.Infof("Watching custom resource definition %s", gvr.String())
		stopCh := make(chan struct{})
		go informer.Informer().Run(stopCh)
		cache.WaitForCacheSync(stopCh, informer.Informer().HasSynced)
		c.startedInformers[gvr] = true
	}
	return informer.Lister()
}

func replicasOfCustomResource(cr *unstructured.Unstructured, replicasPath string) (int, error) {
	replicasPath = strings.TrimPrefix(replicasPath, ".") // ignore leading period
	replicas, _, err := unstructured.NestedInt64(cr.UnstructuredContent(), strings.Split(replicasPath, ".")...)
	return int(replicas), err
}
