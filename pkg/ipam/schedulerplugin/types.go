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
	"errors"

	"tkestack.io/galaxy/pkg/ipam/floatingip"
)

type NotSupportedReleasePolicyError error

const (
	deletedAndIPMutablePod         = "deletedAndIPMutablePod"
	deletedAndParentAppNotExistPod = "deletedAndParentAppNotExistPod"
	deletedAndScaledDownAppPod     = "deletedAndScaledDownAppPod"
	deletedAndScaledDownDpPod      = "deletedAndScaledDownDpPod"
)

var (
	NoReplicas          = NotSupportedReleasePolicyError(errors.New("parent workload has no replicas"))
	NotStatefulWorkload = NotSupportedReleasePolicyError(
		errors.New("pod name doesn't match '.*-[0-9]*$', assume its parent is not a stateful workload"))
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
