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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

func (p *FloatingIPPlugin) getReplicasOfDeployment(keyObj *util.KeyObj) (int, error) {
	dp, err := p.DeploymentLister.Deployments(keyObj.Namespace).Get(keyObj.AppName)
	replicas := 0
	if err != nil {
		if !metaErrs.IsNotFound(err) {
			return 0, err
		}
	} else {
		replicas = int(*dp.Spec.Replicas)
	}
	return replicas, nil
}

// unbindDpPod unbind deployment pod
func (p *FloatingIPPlugin) unbindDpPod(pod *corev1.Pod, keyObj *util.KeyObj, policy constant.ReleasePolicy, when string) error {
	// if ipam or secondIPAM failed, we can depend on resync to release ip
	if err := p.unbindDpPodForIPAM(keyObj, p.ipam, policy, when); err != nil {
		return err
	}
	if p.enabledSecondIP(pod) {
		return p.unbindDpPodForIPAM(keyObj, p.secondIPAM, policy, when)
	}
	return nil
}

// unbindDpPod unbind deployment pod
func (p *FloatingIPPlugin) unbindDpPodForIPAM(keyObj *util.KeyObj, ipam floatingip.IPAM, policy constant.ReleasePolicy,
	when string) error {
	key, prefixKey := keyObj.KeyInDB, keyObj.PoolPrefix()
	if policy == constant.ReleasePolicyPodDelete {
		return releaseIP(ipam, key, fmt.Sprintf("%s %s", deletedAndIPMutablePod, when))
	} else if policy == constant.ReleasePolicyNever {
		if key != prefixKey {
			return reserveIP(key, prefixKey, ipam, fmt.Sprintf("never release policy %s", when))
		}
		return nil
	}
	// TODO AppName stored in fip crd is a deployment name for replicasets pods
	// which makes it impossible to support ReleasePolicyImmutable right now
	replicas, err := p.getReplicasOfDeployment(keyObj)
	if err != nil {
		if !metaErrs.IsNotFound(err) {
			return fmt.Errorf("unbind dp pod %s: %w", key, err)
		}
	}
	if replicas == 0 {
		return releaseIP(ipam, key, fmt.Sprintf("%s %s", deletedAndIPMutablePod, when))
	}
	// locks the pool name if it is a pool
	// locks the deployment app name if it isn't a pool
	lockIndex := p.dpLockPool.GetLockIndex([]byte(prefixKey))
	p.dpLockPool.RawLock(lockIndex)
	defer p.dpLockPool.RawUnlock(lockIndex)
	fips, err := ipam.ByPrefix(prefixKey)
	if err != nil {
		return err
	}
	// if num of fips is large than replicas, release exceeded part
	if len(fips) > replicas {
		return releaseIP(ipam, key, fmt.Sprintf("%s %s", deletedAndScaledDownDpPod, when))
	} else {
		if key != prefixKey {
			return reserveIP(key, prefixKey, ipam, fmt.Sprintf("allocated %d <= replicas %d %s", len(fips), replicas, when))
		}
	}
	return nil
}

// getDpReplicas returns replicas, isPoolSizeDefined, error
func (p *FloatingIPPlugin) getDpReplicas(keyObj *util.KeyObj) (int, bool, error) {
	if keyObj.PoolName != "" {
		pool, err := p.PoolLister.Pools("kube-system").Get(keyObj.PoolName)
		if err == nil {
			glog.V(4).Infof("pool %s size %d", pool.Name, pool.Size)
			return pool.Size, true, nil
		} else {
			if !metaErrs.IsNotFound(err) {
				return 0, false, err
			}
			// pool not found, get replicas from deployment
		}
	}
	replicas, err := p.getReplicasOfDeployment(keyObj)
	if err != nil {
		return 0, false, fmt.Errorf("get replicas for %s: %w", keyObj.KeyInDB, err)
	}
	return replicas, false, nil
}
