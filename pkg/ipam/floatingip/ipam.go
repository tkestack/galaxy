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

	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/utils/database"
)

var (
	// ErrNoEnoughIP is error when there is no available floatingIPs
	ErrNoEnoughIP     = fmt.Errorf("no enough available ips left")
	ErrNoFIPForSubnet = fmt.Errorf("no fip configured for subnet")
)

// IPAM interface which implemented by database and kubernetes CRD
type IPAM interface {
	// ConfigurePool init floatingIP pool.
	ConfigurePool([]*FloatingIP) error
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
	// UpdatePolicy update floatingIP's release policy.
	UpdatePolicy(string, net.IP, constant.ReleasePolicy, string) error
	// Release release a given IP.
	Release(string, net.IP) error
	// First returns the first matched IP by key.
	First(string) (*FloatingIPInfo, error) // returns nil,nil if key is not found
	// ByIP transform a given IP to database.FloatingIP struct.
	ByIP(net.IP) (database.FloatingIP, error)
	// ByPrefix filter floatingIPs by prefix key.
	ByPrefix(string) ([]database.FloatingIP, error)
	// ByKeyword returns floatingIP set by a given keyword.
	ByKeyword(string) ([]database.FloatingIP, error)
	// RoutableSubnet returns node's net subnet.
	RoutableSubnet(net.IP) *net.IPNet
	// RoutableSubnet returns node's net subnet.
	QueryRoutableSubnetByKey(key string) ([]string, error)
	// Shutdown shutdowns IPAM.
	Shutdown()
	// Name returns IPAM's name.
	Name() string
}

// FloatingIPInfo is floatingIP information
type FloatingIPInfo struct {
	IPInfo constant.IPInfo
	FIP    database.FloatingIP
}
