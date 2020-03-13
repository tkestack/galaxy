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
)

func ensureIPAMConf(ipam floatingip.IPAM, lastConf *string, newConf string) (bool, error) {
	if newConf == *lastConf {
		glog.V(4).Infof("[%s] floatingip configmap unchanged", ipam.Name())
		return false, nil
	}
	var conf []*floatingip.FloatingIPPool
	if err := json.Unmarshal([]byte(newConf), &conf); err != nil {
		return false, fmt.Errorf("failed to unmarshal configmap val %s to floatingip config", newConf)
	}
	if err := ipam.ConfigurePool(conf); err != nil {
		return false, fmt.Errorf("failed to configure pool: %v", err)
	}
	glog.Infof("[%s] updated floatingip conf from (%s) to (%s)", ipam.Name(), *lastConf, newConf)
	*lastConf = newConf
	return true, nil
}

func allocateInSubnet(ipam floatingip.IPAM, key string, subnet *net.IPNet, policy constant.ReleasePolicy, attr,
	when string) error {
	ip, err := ipam.AllocateInSubnet(key, subnet, policy, attr)
	if err != nil {
		return err
	}
	glog.Infof("[%s] allocated ip %s to pod %s during %s", ipam.Name(), ip.String(), key, when)
	return nil
}

func allocateInSubnetWithKey(ipam floatingip.IPAM, oldK, newK, subnet string, policy constant.ReleasePolicy,
	attr, when string) error {
	if err := ipam.AllocateInSubnetWithKey(oldK, newK, subnet, policy, attr); err != nil {
		return err
	}
	fip, err := ipam.First(newK)
	if err != nil {
		return err
	}
	glog.Infof("[%s] allocated ip %s to %s from %s during %s", ipam.Name(), fip.IPInfo.IP.String(), newK, oldK, when)
	return nil
}

// #lizard forgives
func getAvailableSubnet(ipam floatingip.IPAM, keyObj *util.KeyObj, policy constant.ReleasePolicy, replicas int,
	isPoolSizeDefined bool) (subnets []string, reserve bool, err error) {
	if keyObj.Deployment() && policy != constant.ReleasePolicyPodDelete {
		var ips []floatingip.FloatingIP
		poolPrefix := keyObj.PoolPrefix()
		poolAppPrefix := keyObj.PoolAppPrefix()
		ips, err = ipam.ByPrefix(poolPrefix)
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
				unusedSubnetSet.Insert(ip.Subnet)
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
			return unusedSubnetSet.List(), true, nil
		}
	}
	if subnets, err = ipam.QueryRoutableSubnetByKey(""); err != nil {
		err = fmt.Errorf("failed to query allocatable subnet: %v", err)
		return
	}
	return
}

func (p *FloatingIPPlugin) releaseIP(key string, reason string, pod *corev1.Pod) error {
	if err := releaseIP(p.ipam, key, reason); err != nil {
		return fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	// skip release second ip if not enabled
	if !(p.hasSecondIPConf.Load().(bool)) {
		return nil
	}
	if pod != nil && !wantSecondIP(pod) {
		return nil
	}
	if err := releaseIP(p.secondIPAM, key, reason); err != nil {
		return fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
	}
	return nil
}

func releaseIP(ipam floatingip.IPAM, key string, reason string) error {
	ipInfo, err := ipam.First(key)
	if err != nil {
		return fmt.Errorf("failed to query floating ip of %s: %v", key, err)
	}
	if ipInfo == nil {
		glog.Infof("[%s] release floating ip from %s because of %s, but already been released", ipam.Name(), key,
			reason)
		return nil
	}
	if err := ipam.Release(key, ipInfo.IPInfo.IP.IP); err != nil {
		return fmt.Errorf("failed to release floating ip of %s because of %s: %v", key, reason, err)
	}
	glog.Infof("[%s] released floating ip %s from %s because of %s", ipam.Name(), ipInfo.IPInfo.IP.String(), key,
		reason)
	return nil
}

func (p *FloatingIPPlugin) reserveIP(old, new, reason string, enabledSecondIP bool) error {
	if err := reserveIP(old, new, p.ipam, reason); err != nil {
		return err
	}
	if enabledSecondIP {
		if err := reserveIP(old, new, p.secondIPAM, reason); err != nil {
			return err
		}
	}
	return nil
}

func reserveIP(key, prefixKey string, ipam floatingip.IPAM, reason string) error {
	if err := ipam.ReserveIP(key, prefixKey, getAttr("")); err != nil {
		return fmt.Errorf("[%s] failed to reserve ip from pod %s to %s: %v", ipam.Name(), key, prefixKey, err)
	}
	glog.Infof("[%s] reserved ip from pod %s to %s, because %s", ipam.Name(), key, prefixKey, reason)
	return nil
}

// getNodeSubnetfromIPAM gets node subnet from ipam
func (p *FloatingIPPlugin) getNodeSubnetfromIPAM(node *corev1.Node) (*net.IPNet, error) {
	nodeIP := getNodeIP(node)
	if nodeIP == nil {
		return nil, errors.New("FloatingIPPlugin:UnknowNode")
	}
	if ipNet := p.ipam.RoutableSubnet(nodeIP); ipNet != nil {
		glog.V(4).Infof("node %s %s %s", node.Name, nodeIP.String(), ipNet.String())
		p.nodeSubnet[node.Name] = ipNet
		return ipNet, nil
	} else {
		return nil, errors.New("FloatingIPPlugin:NoFIPConfigNode")
	}
}
