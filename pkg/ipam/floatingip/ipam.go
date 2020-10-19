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

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/util/sets"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/utils/nets"
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
	AllocateSpecificIP(string, net.IP, Attr) error
	// AllocateInSubnet allocate subnet of IPs.
	AllocateInSubnet(string, *net.IPNet, Attr) (net.IP, error)
	// AllocateInSubnetsAndIPRange allocates an ip for each ip range array of the input node subnet.
	AllocateInSubnetsAndIPRange(string, *net.IPNet, [][]nets.IPRange, Attr) ([]net.IP, error)
	// AllocateInSubnetWithKey allocate a floatingIP in given subnet and key.
	AllocateInSubnetWithKey(oldK, newK, subnet string, attr Attr) error
	// ReserveIP can reserve a IP entitled by a terminated pod. Attributes **expect policy attr** will be updated.
	// Returns true if key or attr updated.
	ReserveIP(oldK, newK string, attr Attr) (bool, error)
	// UpdateAttr update floatingIP's release policy and attrs according to ip and key
	UpdateAttr(string, net.IP, Attr) error
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
	// ByKeyAndIPRanges finds an ip for each iprange array by key, and returns all fips
	ByKeyAndIPRanges(string, [][]nets.IPRange) ([]*FloatingIPInfo, error)
	// NodeSubnets returns node's subnet.
	NodeSubnet(net.IP) *net.IPNet
	// NodeSubnetsByKeyAndIPRanges finds an ip for each iprange array by key, and returns their intersection
	// node subnets.
	NodeSubnetsByKeyAndIPRanges(key string, ipranges [][]nets.IPRange) (sets.String, error)
	// Name returns IPAM's name.
	Name() string
	// implements metrics Collector interface
	prometheus.Collector
}

// FloatingIPInfo is floatingIP information
type FloatingIPInfo struct {
	IPInfo constant.IPInfo
	FIP    FloatingIP
}
