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
package floatingip

import (
	"fmt"
	"net"

	"k8s.io/apimachinery/pkg/util/sets"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
)

var (
	// ErrNoEnoughIP is error when there is no available floatingIPs
	ErrNoEnoughIP = fmt.Errorf("no enough available ips left")
)

// IPAM interface which implemented by kubernetes CRD
type IPAM interface {
	// ConfigurePool init floatingIP pool.
	ConfigurePool([]*FloatingIPPool) error
	// ReleaseIPs releases given ips as long as their keys match and returned released and unreleased map
	// released and unreleased map are guaranteed to be none nil even if err is not nil
	// unreleased map stores ip with its latest key if key changed
	ReleaseIPs(map[string]string) (map[string]string, map[string]string, error)
	// AllocateSpecificIP allocate pod a specific IP.
	AllocateSpecificIP(string, net.IP, constant.ReleasePolicy, string) error
	// AllocateInSubnet allocate subnet of IPs.
	AllocateInSubnet(string, *net.IPNet, constant.ReleasePolicy, string) (net.IP, error)
	// AllocateInSubnetWithKey allocate a floatingIP in given subnet and key.
	AllocateInSubnetWithKey(oldK, newK, subnet string, policy constant.ReleasePolicy, attr string) error
	// ReserveIP can reserve a IP entitled by a terminated pod.
	ReserveIP(oldK, newK, attr string) error
	// UpdatePolicy update floatingIP's release policy and attr according to ip and key
	UpdatePolicy(string, net.IP, constant.ReleasePolicy, string) error
	// Release release a given IP.
	Release(string, net.IP) error
	// First returns the first matched IP by key.
	First(string) (*FloatingIPInfo, error) // returns nil,nil if key is not found
	// ByIP transform a given IP to FloatingIP struct.
	ByIP(net.IP) (FloatingIP, error)
	// ByPrefix filter floatingIPs by prefix key.
	ByPrefix(string) ([]FloatingIP, error)
	// ByKeyword returns floatingIP set by a given keyword.
	ByKeyword(string) ([]FloatingIP, error)
	// NodeSubnets returns node's subnet.
	NodeSubnet(net.IP) *net.IPNet
	// NodeSubnetsByKey returns keys corresponding node subnets which has `key` as a prefix.
	NodeSubnetsByKey(key string) (sets.String, error)
	// Name returns IPAM's name.
	Name() string
}

// FloatingIPInfo is floatingIP information
type FloatingIPInfo struct {
	IPInfo constant.IPInfo
	FIP    FloatingIP
}
