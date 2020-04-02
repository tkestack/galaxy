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
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	crd_clientset "tkestack.io/galaxy/pkg/ipam/client/clientset/versioned"
	"tkestack.io/galaxy/pkg/utils/nets"
)

// Type is struct of IP type.
type Type uint16

const (
	// InternalIp is enum of pod's internal IP.
	InternalIp Type = iota
	// ExternalIp is enum of pod's external IP.
	ExternalIp
)

// String used to transform IP Type to string.
func (t *Type) String() (string, error) {
	if *t == InternalIp {
		return "internalIP", nil
	} else if *t == ExternalIp {
		return "externalIP", nil
	}
	return "", fmt.Errorf("unknown ip type %v", *t)
}

// FloatingIPObj stores floatingIP object information.
type FloatingIPObj struct {
	key        string
	att        string
	policy     constant.ReleasePolicy
	subnetSet  sets.String
	updateTime time.Time
}

// FIP is cache of floatingIP, key is FloatingIP name (ip typed as uint32)
// value stores FloatingIPSpec in FloatingIP CRD.
type FIPCache struct {
	cacheLock       *sync.RWMutex
	allocatedFIPs   map[string]*FloatingIPObj
	unallocatedFIPs map[string]*FloatingIPObj
}

type crdIpam struct {
	FloatingIPs []*FloatingIPPool `json:"floatingips,omitempty"`
	client      crd_clientset.Interface
	ipType      Type
	//caches for FloatingIP crd, both stores allocated FloatingIPs and unallocated FloatingIPs
	caches FIPCache
}

// NewCrdIPAM init IPAM struct.
func NewCrdIPAM(fipClient crd_clientset.Interface, ipType Type) IPAM {
	ipam := &crdIpam{
		client: fipClient,
		ipType: ipType,
	}
	ipam.caches.cacheLock = new(sync.RWMutex)
	return ipam
}

// ConfigurePool init floatingIP pool.
func (ci *crdIpam) ConfigurePool(floatIPs []*FloatingIPPool) error {
	if err := ci.freshCache(floatIPs); err != nil {
		return err
	}
	return nil
}

// AllocateSpecificIP allocate pod a specific IP.
func (ci *crdIpam) AllocateSpecificIP(key string, ip net.IP, policy constant.ReleasePolicy, attr string) error {
	ipStr := ip.String()
	ci.caches.cacheLock.RLock()
	spec, find := ci.caches.unallocatedFIPs[ipStr]
	ci.caches.cacheLock.RUnlock()
	if !find {
		return fmt.Errorf("failed to find floating ip by %s in cache", ipStr)
	}
	date := time.Now()
	if err := ci.createFloatingIP(ipStr, key, policy, attr, spec.subnetSet, date); err != nil {
		glog.Errorf("failed to create floatingIP %s: %v", ipStr, err)
		return err
	}
	ci.caches.cacheLock.Lock()
	ci.syncCacheAfterCreate(ipStr, key, attr, policy, spec.subnetSet, date)
	ci.caches.cacheLock.Unlock()
	return nil
}

// AllocateInSubnet allocate subnet of IPs.
func (ci *crdIpam) AllocateInSubnet(key string, nodeSubnet *net.IPNet, policy constant.ReleasePolicy,
	attr string) (allocated net.IP, err error) {
	if nodeSubnet == nil {
		// this should never happen
		return nil, fmt.Errorf("nil nodeSubnet")
	}
	var ipStr string
	ci.caches.cacheLock.Lock()
	nodeSubnetStr := nodeSubnet.String()
	for k, v := range ci.caches.unallocatedFIPs {
		//find an unallocated fip, then use it
		if v.subnetSet.Has(nodeSubnetStr) {
			ipStr = k
			date := time.Now()
			if err = ci.createFloatingIP(ipStr, key, policy, attr, v.subnetSet, date); err != nil {
				glog.Errorf("failed to create floatingIP %s: %v", ipStr, err)
				ci.caches.cacheLock.Unlock()
				return
			}
			//sync cache when crd create success
			ci.syncCacheAfterCreate(ipStr, key, attr, policy, v.subnetSet, date)
			break
		}
	}
	ci.caches.cacheLock.Unlock()
	if ipStr == "" {
		return nil, ErrNoEnoughIP
	}
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	if err = ci.getFloatingIP(ipStr); err != nil {
		return
	}
	allocated = net.ParseIP(ipStr)
	return
}

// AllocateInSubnetWithKey allocate a floatingIP in given subnet and key.
func (ci *crdIpam) AllocateInSubnetWithKey(oldK, newK, subnet string, policy constant.ReleasePolicy,
	attr string) error {
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	var (
		recordTs int64
		recordIP string
		latest   *FloatingIPObj
	)
	//find latest floatingIP by updateTime.
	for k, v := range ci.caches.allocatedFIPs {
		if v.key == oldK && v.subnetSet.Has(subnet) {
			if v.updateTime.UnixNano() > recordTs {
				recordIP = k
				latest = v
				recordTs = v.updateTime.UnixNano()
			}
		}
	}
	if latest == nil {
		return fmt.Errorf("failed to find floatIP by key %s", oldK)
	}
	date := time.Now()
	if err := ci.updateFloatingIP(recordIP, newK, sets.NewString(subnet), policy, attr, date); err != nil {
		glog.Errorf("failed to update floatingIP %s: %v", recordIP, err)
		return err
	}
	latest.key = newK
	latest.updateTime = date
	latest.policy = policy
	latest.att = attr
	return nil
}

// ReserveIP can reserve a IP entitled by a terminated pod.
func (ci *crdIpam) ReserveIP(oldK, newK, attr string) error {
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	for k, v := range ci.caches.allocatedFIPs {
		if v.key == oldK {
			date := time.Now()
			if err := ci.updateFloatingIP(k, newK, v.subnetSet, v.policy, attr, date); err != nil {
				glog.Errorf("failed to update floatingIP %s: %v", k, err)
				return err
			}
			v.key = newK
			v.updateTime = date
			v.att = attr
			return nil
		}
	}
	return fmt.Errorf("failed to find floatIP by key %s", oldK)
}

// UpdatePolicy update floatingIP's release policy.
func (ci *crdIpam) UpdatePolicy(key string, ip net.IP, policy constant.ReleasePolicy, attr string) error {
	ipStr := ip.String()
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	v, find := ci.caches.allocatedFIPs[ipStr]
	if !find {
		return fmt.Errorf("failed to find floatIP in cache by IP %s", ipStr)
	}
	date := time.Now()
	if err := ci.updateFloatingIP(ipStr, key, v.subnetSet, policy, attr, date); err != nil {
		glog.Errorf("failed to update floatingIP %s: %v", ipStr, err)
		return err
	}
	v.policy = policy
	v.att = attr
	v.updateTime = date
	return nil
}

// Release release a given IP.
func (ci *crdIpam) Release(key string, ip net.IP) error {
	ipStr := ip.String()
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	v, find := ci.caches.allocatedFIPs[ipStr]
	if !find {
		return fmt.Errorf("failed to find floatIP in cache by IP %s", ipStr)
	}
	if v.key != key {
		return fmt.Errorf("key in %s is %s, not %s", ipStr, v.key, key)
	}
	if err := ci.deleteFloatingIP(ipStr); err != nil {
		return err
	}
	ci.syncCacheAfterDel(ipStr)
	return nil
}

// First returns the first matched IP by key.
func (ci *crdIpam) First(key string) (*FloatingIPInfo, error) {
	fip, err := ci.findFloatingIPByKey(key)
	if err != nil {
		return nil, err
	}
	if fip.Key == "" {
		return nil, nil
	}
	for _, fips := range ci.FloatingIPs {
		if fips.Contains(fip.IP) {
			ip := nets.IPNet(net.IPNet{
				IP:   fip.IP,
				Mask: fips.Mask,
			})
			return &FloatingIPInfo{
				IPInfo: constant.IPInfo{
					IP:      &ip,
					Vlan:    fips.Vlan,
					Gateway: fips.Gateway,
				},
				FIP: fip,
			}, nil
		}
	}
	return nil, fmt.Errorf("could not find match floating ip config for ip %s", fip.IP.String())
}

// ByIP transform a given IP to FloatingIP struct.
func (ci *crdIpam) ByIP(ip net.IP) (FloatingIP, error) {
	ipStr := ip.String()
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	v, find := ci.caches.allocatedFIPs[ipStr]
	if !find {
		v, find = ci.caches.unallocatedFIPs[ipStr]
		if !find {
			return FloatingIP{}, nil
		}
	}
	return convert(ip.String(), v), nil
}

// ByPrefix filter floatingIPs by prefix key.
func (ci *crdIpam) ByPrefix(prefix string) ([]FloatingIP, error) {
	var fips []FloatingIP
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	for ip, spec := range ci.caches.allocatedFIPs {
		if strings.HasPrefix(spec.key, prefix) {
			fips = append(fips, convert(ip, spec))
		}
	}
	if prefix == "" {
		for ip, spec := range ci.caches.unallocatedFIPs {
			fips = append(fips, convert(ip, spec))
		}
	}
	return fips, nil
}

func convert(ip string, spec *FloatingIPObj) FloatingIP {
	return FloatingIP{
		Key:       spec.key,
		Subnets:   spec.subnetSet,
		Attr:      spec.att,
		Policy:    uint16(spec.policy),
		IP:        net.ParseIP(ip),
		UpdatedAt: spec.updateTime,
	}
}

func (ci *crdIpam) NodeSubnet(nodeIP net.IP) *net.IPNet {
	for j := range ci.FloatingIPs {
		nodeSubnets := ci.FloatingIPs[j].NodeSubnets
		for k := range nodeSubnets {
			if nodeSubnets[k].Contains(nodeIP) {
				return nodeSubnets[k]
			}
		}
	}
	return nil
}

func (ci *crdIpam) NodeSubnetsByKey(key string) (sets.String, error) {
	if key == "" {
		return ci.filterUnallocatedSubnet(), nil
	}
	return ci.filterAllocatedSubnet(key), nil
}

// Shutdown shutdowns IPAM.
func (ci *crdIpam) Shutdown() {
}

// Name returns IPAM's name.
func (ci *crdIpam) Name() string {
	name, err := ci.ipType.String()
	if err != nil {
		return "unknown type"
	}
	return name
}

// #lizard forgives
func (ci *crdIpam) freshCache(floatIPs []*FloatingIPPool) error {
	glog.V(3).Infof("begin to fresh cache")
	sort.Sort(FloatingIPSlice(floatIPs))
	ips, err := ci.listFloatingIPs()
	if err != nil {
		glog.Errorf("fail to list floatIP %v", err)
		return err
	}
	glog.V(3).Infof("floating ip config %v", floatIPs)
	nodeSubnets := make([]sets.String, len(floatIPs))
	for i, fipConf := range floatIPs {
		subnetSet := sets.NewString()
		for i := range fipConf.NodeSubnets {
			subnetSet.Insert(fipConf.NodeSubnets[i].String())
		}
		nodeSubnets[i] = subnetSet
	}
	var deletingIPs []string
	tmpCacheAllocated := make(map[string]*FloatingIPObj)
	//delete no longer available floating ips stored in etcd first
	for _, ip := range ips.Items {
		netIP := net.ParseIP(ip.Name)
		found := false
		for i, fipConf := range floatIPs {
			if fipConf.IPNet().Contains(netIP) && fipConf.Contains(netIP) {
				found = true
				//ip in config, insert it into cache
				tmpFip := &FloatingIPObj{
					key:        ip.Spec.Key,
					att:        ip.Spec.Attribute,
					policy:     ip.Spec.Policy,
					subnetSet:  nodeSubnets[i],
					updateTime: ip.Spec.UpdateTime.Time,
				}
				tmpCacheAllocated[ip.Name] = tmpFip
				break
			}
		}
		if !found {
			deletingIPs = append(deletingIPs, ip.Name)
		}
	}
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	ci.FloatingIPs = floatIPs
	ci.caches.allocatedFIPs = tmpCacheAllocated
	if len(deletingIPs) > 0 {
		for _, ip := range deletingIPs {
			if err := ci.deleteFloatingIP(ip); err != nil {
				//if a FloatingIP crd in etcd can't be deleted, every freshCache will produce an error
				//it won't return error when error happens in deletion
				glog.Errorf("failed to delete ip %v: %v", ip, err)
			}
		}
		glog.Infof("expect to delete %d ips from %v", len(deletingIPs), deletingIPs)
	}
	now := time.Now()
	// fresh unallocated floatIP
	tmpCacheUnallocated := make(map[string]*FloatingIPObj)
	for i, fipConf := range floatIPs {
		subnetSet := nodeSubnets[i]
		for _, ipr := range fipConf.IPRanges {
			first := nets.IPToInt(ipr.First)
			last := nets.IPToInt(ipr.Last)
			for ; first <= last; first++ {
				ipStr := nets.IntToIP(first).String()
				if _, contain := ci.caches.allocatedFIPs[ipStr]; !contain {
					tmpFip := &FloatingIPObj{
						key:        "",
						att:        "",
						policy:     constant.ReleasePolicyPodDelete,
						subnetSet:  subnetSet,
						updateTime: now,
					}
					tmpCacheUnallocated[ipStr] = tmpFip
				}
			}
		}
	}
	ci.caches.unallocatedFIPs = tmpCacheUnallocated
	return nil
}

// cacheLock is used when the function called,
// don't use lock inner function, otherwise deadlock will be caused
func (ci *crdIpam) syncCacheAfterCreate(ip string, key string, att string, policy constant.ReleasePolicy,
	subnetSet sets.String, date time.Time) {
	tmp := &FloatingIPObj{
		key:        key,
		att:        att,
		policy:     policy,
		subnetSet:  subnetSet,
		updateTime: date,
	}
	ci.caches.allocatedFIPs[ip] = tmp
	delete(ci.caches.unallocatedFIPs, ip)
	return
}

// CacheLock will be used when syncCacheAfterDel called,
// don't use lock inner function, otherwise deadlock will be caused
func (ci *crdIpam) syncCacheAfterDel(ip string) {
	tmp := &FloatingIPObj{
		key:        "",
		att:        "",
		policy:     constant.ReleasePolicyPodDelete,
		subnetSet:  ci.caches.allocatedFIPs[ip].subnetSet,
		updateTime: time.Now(),
	}
	delete(ci.caches.allocatedFIPs, ip)
	ci.caches.unallocatedFIPs[ip] = tmp
	return
}

func (ci *crdIpam) findFloatingIPByKey(key string) (FloatingIP, error) {
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	for ip, spec := range ci.caches.allocatedFIPs {
		if spec.key == key {
			return convert(ip, spec), nil
		}
	}
	return FloatingIP{}, nil
}

func (ci *crdIpam) filterAllocatedSubnet(key string) sets.String {
	//key would not be empty
	subnetSet := sets.NewString()
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	for _, spec := range ci.caches.allocatedFIPs {
		if spec.key == key {
			subnetSet.Insert(spec.subnetSet.List()...)
		}
	}
	return subnetSet
}

// Sometimes unallocated subnet(key equals "") is needed,
// it will filter all subnet in unallocated floatingIP in cache
func (ci *crdIpam) filterUnallocatedSubnet() sets.String {
	subnetSet := sets.NewString()
	ci.caches.cacheLock.RLock()
	for _, val := range ci.caches.unallocatedFIPs {
		subnetSet.Insert(val.subnetSet.List()...)
	}
	ci.caches.cacheLock.RUnlock()
	return subnetSet
}

// ByKeyword returns floatingIP set by a given keyword.
func (ci *crdIpam) ByKeyword(keyword string) ([]FloatingIP, error) {
	//not implement
	var fips []FloatingIP
	ci.caches.cacheLock.RLock()
	defer ci.caches.cacheLock.RUnlock()
	if ci.caches.allocatedFIPs == nil {
		return fips, nil
	}
	for ip, spec := range ci.caches.allocatedFIPs {
		if strings.Contains(spec.key, keyword) {
			fips = append(fips, convert(ip, spec))
		}
	}
	return fips, nil
}

// ReleaseIPs function release a map of ip to key
func (ci *crdIpam) ReleaseIPs(ipToKey map[string]string) (map[string]string, map[string]string, error) {
	deleted, undeleted := map[string]string{}, map[string]string{}
	ci.caches.cacheLock.Lock()
	defer ci.caches.cacheLock.Unlock()
	for ipStr, key := range ipToKey {
		undeleted[ipStr] = key
	}
	if ci.caches.allocatedFIPs == nil {
		//for second ipam, caches may be nil
		return deleted, undeleted, nil
	}
	for ipStr, key := range ipToKey {
		if v, find := ci.caches.allocatedFIPs[ipStr]; find {
			if v.key == key {
				if err := ci.deleteFloatingIP(ipStr); err != nil {
					glog.Errorf("failed to delete %v", ipStr)
					return deleted, undeleted, fmt.Errorf("failed to delete %v", ipStr)
				}
				ci.syncCacheAfterDel(ipStr)
				glog.Infof("%v has been deleted", ipStr)
				deleted[ipStr] = key
				delete(undeleted, ipStr)
			} else {
				// update key
				undeleted[ipStr] = v.key
			}
		} else if _, find := ci.caches.unallocatedFIPs[ipStr]; find {
			undeleted[ipStr] = ""
		}
	}
	return deleted, undeleted, nil
}
