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
package constant

import (
	"encoding/json"
	"fmt"
	"net"

	"tkestack.io/galaxy/pkg/utils/nets"
)

const (
	// cni args in pod's annotation
	ExtendedCNIArgsAnnotation = "k8s.v1.cni.galaxy.io/args"

	MultusCNIAnnotation = "k8s.v1.cni.cncf.io/networks"

	CommonCNIArgsKey = "common"
)

// ParseExtendedCNIArgs parses extended cni args from pod annotation
func ParseExtendedCNIArgs(args string) (map[string]map[string]json.RawMessage, error) {
	argsMap := map[string]map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(args), &argsMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s value %s: %v", ExtendedCNIArgsAnnotation, args, err)
	}
	return argsMap, nil
}

const (
	IPInfosKey = "ipinfos"
)

// IPInfo is the container ip info
type IPInfo struct {
	IP             *nets.IPNet `json:"ip"`
	Vlan           uint16      `json:"vlan"`
	Gateway        net.IP      `json:"gateway"`
	RoutableSubnet *nets.IPNet `json:"routable_subnet"` //the node subnet
}

// FormatIPInfo formats ipInfos as extended CNI Args annotation value
func FormatIPInfo(ipInfos []IPInfo) (string, error) {
	data, err := json.Marshal(ipInfos)
	if err != nil {
		return "", err
	}
	raw := json.RawMessage(data)
	str, err := json.Marshal(map[string]map[string]*json.RawMessage{CommonCNIArgsKey: {IPInfosKey: &raw}})
	return string(str), err
}

// ParseIPInfo pareses ipInfo from annotation
func ParseIPInfo(str string) ([]IPInfo, error) {
	m := map[string]map[string]*json.RawMessage{}
	if err := json.Unmarshal([]byte(str), &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s value %s: %v", ExtendedCNIArgsAnnotation, str, err)
	}
	commonMap := m[CommonCNIArgsKey]
	if commonMap == nil {
		return []IPInfo{}, nil
	}
	ipInfoStr := commonMap[IPInfosKey]
	if ipInfoStr == nil {
		return []IPInfo{}, nil
	}
	var ipInfos []IPInfo
	if err := json.Unmarshal([]byte(*ipInfoStr), &ipInfos); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s value %s as common/ipInfos: %v", ExtendedCNIArgsAnnotation, str, err)
	}
	return ipInfos, nil
}

// ReleasePolicy defines floatingip release policy
type ReleasePolicy uint16

const (
	ReleasePolicyPodDelete ReleasePolicy = iota // release ip as soon as possible
	ReleasePolicyImmutable
	ReleasePolicyNever
)

const (
	ReleasePolicyAnnotation = "k8s.v1.cni.galaxy.io/release-policy"
	Immutable               = "immutable" // Release IP Only when deleting or scale down App
	Never                   = "never"     // Never Release IP
)

func ConvertReleasePolicy(policyStr string) ReleasePolicy {
	switch policyStr {
	case Never:
		return ReleasePolicyNever
	case Immutable:
		return ReleasePolicyImmutable
	default:
		return ReleasePolicyPodDelete
	}
}

const (
	ResourceKind = "FloatingIP"
	ApiVersion   = "galaxy.k8s.io/v1alpha1"
	NameSpace    = "floating-ip"
	IpType       = "ipType"
)
