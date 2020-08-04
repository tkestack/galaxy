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
	extensionClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	appv1 "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	crd_clientset "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned"
	crdInformer "tkestack.io/galaxy/pkg/ipam/client/informers/externalversions/galaxy/v1alpha1"
	list "tkestack.io/galaxy/pkg/ipam/client/listers/galaxy/v1alpha1"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/tapp/pkg/client/clientset/versioned"
	"tkestack.io/tapp/pkg/client/listers/tappcontroller/v1"
)

type PluginFactoryArgs struct {
	Client            kubernetes.Interface
	TAppClient        versioned.Interface
	PodLister         corev1lister.PodLister
	StatefulSetLister appv1.StatefulSetLister
	DeploymentLister  appv1.DeploymentLister
	TAppLister        v1.TAppLister
	PoolLister        list.PoolLister
	PodHasSynced      func() bool
	StatefulSetSynced func() bool
	DeploymentSynced  func() bool
	TAppHasSynced     func() bool
	PoolSynced        func() bool
	CrdClient         crd_clientset.Interface
	ExtClient         extensionClient.Interface
	FIPInformer       crdInformer.FloatingIPInformer
}

const (
	deletedAndIPMutablePod         = "deletedAndIPMutablePod"
	deletedAndParentAppNotExistPod = "deletedAndParentAppNotExistPod"
	deletedAndScaledDownAppPod     = "deletedAndScaledDownAppPod"
	deletedAndScaledDownDpPod      = "deletedAndScaledDownDpPod"
)

type Conf struct {
	FloatingIPs           []*floatingip.FloatingIPPool `json:"floatingips,omitempty"`
	ResyncInterval        uint                         `json:"resyncInterval"`
	ConfigMapName         string                       `json:"configMapName"`
	ConfigMapNamespace    string                       `json:"configMapNamespace"`
	FloatingIPKey         string                       `json:"floatingipKey"` // configmap floatingip data key
	CloudProviderGRPCAddr string                       `json:"cloudProviderGrpcAddr"`
}

func (conf *Conf) validate() {
	if conf.ResyncInterval < 1 {
		conf.ResyncInterval = 1
	}
	if conf.ConfigMapName == "" {
		conf.ConfigMapName = "floatingip-config"
	}
	if conf.ConfigMapNamespace == "" {
		conf.ConfigMapNamespace = "kube-system"
	}
	if conf.FloatingIPKey == "" {
		conf.FloatingIPKey = "floatingips"
	}
}
