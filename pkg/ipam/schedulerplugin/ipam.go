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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	"tkestack.io/galaxy/pkg/utils/nets"
)

func (p *FloatingIPPlugin) ensureIPAMConf(lastConf *string, newConf string) (bool, error) {
	if newConf == *lastConf {
		glog.V(4).Infof("floatingip configmap unchanged")
		return false, nil
	}
	var conf []*floatingip.FloatingIPPool
	if err := json.Unmarshal([]byte(newConf), &conf); err != nil {
		return false, fmt.Errorf("failed to unmarshal configmap val %s to floatingip config: %v", newConf, err)
	}
	if err := p.ipam.ConfigurePool(conf); err != nil {
		return false, fmt.Errorf("failed to configure pool: %v", err)
	}
	glog.Infof("updated floatingip conf from (%s) to (%s)", *lastConf, newConf)
	*lastConf = newConf
	return true, nil
}

func (p *FloatingIPPlugin) allocateInSubnet(key string, subnet *net.IPNet, attr floatingip.Attr, when string) error {
	ip, err := p.ipam.AllocateInSubnet(key, subnet, attr)
	if err != nil {
		return err
	}
	glog.Infof("allocated ip %s to pod %s during %s", ip.String(), key, when)
	return nil
}

func (p *FloatingIPPlugin) allocateInSubnetWithKey(oldK, newK, subnet string, attr floatingip.Attr, when string) error {
	if err := p.ipam.AllocateInSubnetWithKey(oldK, newK, subnet, attr); err != nil {
		return err
	}
	fip, err := p.ipam.First(newK)
	if err != nil {
		return err
	}
	glog.Infof("allocated ip %s to %s from %s during %s", fip.IPInfo.IP.String(), newK, oldK, when)
	return nil
}

// #lizard forgives
func (p *FloatingIPPlugin) getAvailableSubnet(keyObj *util.KeyObj, policy constant.ReleasePolicy, replicas int,
	isPoolSizeDefined bool, ipranges [][]nets.IPRange) (subnets sets.String, reserve bool, err error) {
	if keyObj.Deployment() && policy != constant.ReleasePolicyPodDelete {
		if len(ipranges) > 0 {
			// this introduce lots of complexity, don't support it for now
			return nil, false, fmt.Errorf("request ip ranges for deployment pod with release " +
				"policy other than ReleasePolicyPodDelete is not supported")
		}
		var ips []*floatingip.FloatingIPInfo
		poolPrefix := keyObj.PoolPrefix()
		poolAppPrefix := keyObj.PoolAppPrefix()
		ips, err = p.ipam.ByPrefix(poolPrefix)
		if err != nil {
			err = fmt.Errorf("failed query prefix %s: %s", poolPrefix, err)
			return
		}
		usedCount := 0
		unusedSubnetSet := sets.NewString()
		for _, ip := range ips {
			if ip.Key != poolPrefix {
				if isPoolSizeDefined || keyObj.PoolName == "" {
					usedCount++
				} else {
					if strings.HasPrefix(ip.Key, poolAppPrefix) {
						// Don't counting in other deployments' used ip if sharing pool
						usedCount++
					}
				}
			} else {
				unusedSubnetSet.Insert(ip.NodeSubnets.UnsortedList()...)
			}
		}
		glog.V(4).Infof("keyObj %v, unusedSubnetSet %v, usedCount %d, replicas %d, isPoolSizeDefined %v", keyObj,
			unusedSubnetSet, usedCount, replicas, isPoolSizeDefined)
		// check usedCount >= replicas to ensure upgrading a deployment won't change its ips
		if usedCount >= replicas {
			if isPoolSizeDefined {
				return nil, false, fmt.Errorf("reached pool %s size limit of %d", keyObj.PoolName, replicas)
			}
			return nil, false, fmt.Errorf("deployment %s has allocated %d ips with replicas of %d, wait for releasing",
				keyObj.AppName, usedCount, replicas)
		}
		if unusedSubnetSet.Len() > 0 {
			return unusedSubnetSet, true, nil
		}
	}
	if subnets, err = p.ipam.NodeSubnetsByIPRanges(ipranges); err != nil {
		err = fmt.Errorf("failed to query allocatable subnet: %v", err)
		return
	}
	return
}

func (p *FloatingIPPlugin) releaseIP(key string, reason string) error {
	ipInfos, err := p.ipam.ByKeyAndIPRanges(key, nil)
	if len(ipInfos) == 0 {
		glog.Infof("release floating ip from %s because of %s, but already been released", key, reason)
		return nil
	}
	m := map[string]string{}
	for i := range ipInfos {
		m[ipInfos[i].IP.String()] = ipInfos[i].Key
	}
	released, unreleased, err := p.ipam.ReleaseIPs(m)
	if err != nil {
		return fmt.Errorf("released %v, unreleased %v of %s because of %s: %v", released, unreleased, key,
			reason, err)
	}
	glog.Infof("released floating ip %v from %s because of %s", released, key, reason)
	return nil
}

func (p *FloatingIPPlugin) reserveIP(key, prefixKey string, reason string) error {
	if reserved, err := p.ipam.ReserveIP(key, prefixKey, floatingip.Attr{}); err != nil {
		return fmt.Errorf("reserve ip from pod %s to %s: %v", key, prefixKey, err)
	} else if reserved {
		// resync will call reserveIP for sts and tapp pod with immutable/never release policy for endless times,
		// so print "success reserved ip" only when succeeded
		glog.Infof("reserved ip from pod %s to %s, because %s", key, prefixKey, reason)
	}
	return nil
}

// getNodeSubnetfromIPAM gets node subnet from ipam
func (p *FloatingIPPlugin) getNodeSubnetfromIPAM(node *corev1.Node) (*net.IPNet, error) {
	nodeIP := getNodeIP(node)
	if nodeIP == nil {
		return nil, errors.New("FloatingIPPlugin:UnknowNode")
	}
	if ipNet := p.ipam.NodeSubnet(nodeIP); ipNet != nil {
		glog.V(4).Infof("node %s %s %s", node.Name, nodeIP.String(), ipNet.String())
		p.nodeSubnet[node.Name] = ipNet
		return ipNet, nil
	} else {
		return nil, errors.New("FloatingIPPlugin:NoFIPConfigNode")
	}
}
