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

	// For fip crd object which has this label, it's reserved by admin manually. IPAM will not allocate it to pods.
	ReserveFIPLabel = "reserved"

	IPInfosKey = "ipinfos"
)

// IPInfo is the container ip info
type IPInfo struct {
	IP      *nets.IPNet `json:"ip"`
	Vlan    uint16      `json:"vlan"`
	Gateway net.IP      `json:"gateway"`
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

func PolicyStr(policy ReleasePolicy) string {
	return [...]string{"", Immutable, Never}[policy]
}

const (
	ResourceKind = "FloatingIP"
	ApiVersion   = "galaxy.k8s.io/v1alpha1"
	NameSpace    = "floating-ip"
	IpType       = "ipType"
)

// CniArgs is the cni args in pod annotation
type CniArgs struct {
	// RequestIPRange is the requested ip candidates to allocate, one ip per []nets.IPRange
	RequestIPRange [][]nets.IPRange `json:"request_ip_range,omitempty"`
	// Common is the common args for cni plugins to setup network
	Common CommonCniArgs `json:"common"`
}

type CommonCniArgs struct {
	IPInfos []IPInfo `json:"ipinfos,omitempty"`
}

// UnmarshalCniArgs unmarshal cni args from input str
func UnmarshalCniArgs(str string) (*CniArgs, error) {
	if str == "" {
		return nil, nil
	}
	var cniArgs CniArgs
	if err := json.Unmarshal([]byte(str), &cniArgs); err != nil {
		return nil, fmt.Errorf("unmarshal pod cni args: %v", err)
	}
	return &cniArgs, nil
}

// MarshalCniArgs marshal cni args of the given ipInfos
func MarshalCniArgs(ipInfos []IPInfo) (string, error) {
	cniArgs := CniArgs{Common: CommonCniArgs{
		IPInfos: ipInfos,
	}}
	data, err := json.Marshal(cniArgs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
