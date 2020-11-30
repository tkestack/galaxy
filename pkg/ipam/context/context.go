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
package context

import (
	"time"

	extensionClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	extensioninformer "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	extensionlister "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	coreinformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	appv1 "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	crd_clientset "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned"
	crdInformer "tkestack.io/galaxy/pkg/ipam/client/informers/externalversions"
	galaxyinformer "tkestack.io/galaxy/pkg/ipam/client/informers/externalversions/galaxy/v1alpha1"
	list "tkestack.io/galaxy/pkg/ipam/client/listers/galaxy/v1alpha1"
)

// IPAMContext has k8s client, lister and informer fields
type IPAMContext struct {
	Client        kubernetes.Interface
	GalaxyClient  crd_clientset.Interface
	ExtClient     extensionClient.Interface
	DynamicClient dynamic.Interface

	PodLister         corev1lister.PodLister
	StatefulSetLister appv1.StatefulSetLister
	DeploymentLister  appv1.DeploymentLister
	PoolLister        list.PoolLister
	ExtensionLister   extensionlister.CustomResourceDefinitionLister

	PodInformer coreinformer.PodInformer
	FIPInformer galaxyinformer.FloatingIPInformer

	informerFactory    informers.SharedInformerFactory
	crdInformerFactory crdInformer.SharedInformerFactory
	extensionFactory   extensioninformer.SharedInformerFactory
}

// NewIPAMContext creates a new IPAMContext given the clients
func NewIPAMContext(client kubernetes.Interface, galaxyClient crd_clientset.Interface,
	extClient extensionClient.Interface, DynamicClient dynamic.Interface) *IPAMContext {
	ctx := &IPAMContext{
		Client:        client,
		GalaxyClient:  galaxyClient,
		ExtClient:     extClient,
		DynamicClient: DynamicClient,
	}
	ctx.informerFactory = informers.NewSharedInformerFactoryWithOptions(ctx.Client, time.Minute)
	ctx.PodInformer = ctx.informerFactory.Core().V1().Pods()
	statefulsetInformer := ctx.informerFactory.Apps().V1().StatefulSets()
	deploymentInformer := ctx.informerFactory.Apps().V1().Deployments()
	ctx.crdInformerFactory = crdInformer.NewSharedInformerFactory(ctx.GalaxyClient, 0)
	poolInformer := ctx.crdInformerFactory.Galaxy().V1alpha1().Pools()
	ctx.FIPInformer = ctx.crdInformerFactory.Galaxy().V1alpha1().FloatingIPs()
	ctx.extensionFactory = extensioninformer.NewSharedInformerFactory(ctx.ExtClient, 0)
	extensionInformer := ctx.extensionFactory.Apiextensions().V1beta1().CustomResourceDefinitions()
	extensionInformer.Informer() // call Informer to actually create an informer

	ctx.PodLister = ctx.PodInformer.Lister()
	ctx.StatefulSetLister = statefulsetInformer.Lister()
	ctx.DeploymentLister = deploymentInformer.Lister()
	ctx.PoolLister = poolInformer.Lister()

	ctx.ExtensionLister = extensionInformer.Lister()
	return ctx
}

// StartInformers starts the informer factories and wait for cache sync
func (ctx *IPAMContext) StartInformers(stopChan chan struct{}) {
	ctx.informerFactory.Start(stopChan)
	ctx.crdInformerFactory.Start(stopChan)
	ctx.extensionFactory.Start(stopChan)
	ctx.informerFactory.WaitForCacheSync(stopChan)
	ctx.crdInformerFactory.WaitForCacheSync(stopChan)
	ctx.extensionFactory.WaitForCacheSync(stopChan)
}
