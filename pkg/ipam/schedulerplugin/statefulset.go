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

	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

func (p *FloatingIPPlugin) unbindNoneDpPod(keyObj *util.KeyObj, policy constant.ReleasePolicy, when string) error {
	key := keyObj.KeyInDB
	if policy == constant.ReleasePolicyPodDelete || p.supportReserveIPPolicy(keyObj, policy) != nil {
		return p.releaseIP(key, fmt.Sprintf("%s %s", deletedAndIPMutablePod, when))
	} else if policy == constant.ReleasePolicyNever {
		return p.reserveIP(key, key, fmt.Sprintf("never release policy %s", when))
	} else if policy == constant.ReleasePolicyImmutable {
		appExist, replicas, err := p.checkAppAndReplicas(keyObj)
		if err != nil {
			return err
		}
		shouldRelease, reason, err := p.shouldRelease(keyObj, appExist, replicas)
		if err != nil {
			return err
		}
		reason = fmt.Sprintf("%s %s", reason, when)
		if !shouldRelease {
			return p.reserveIP(key, key, reason)
		} else {
			return p.releaseIP(key, reason)
		}
	}
	return nil
}

func (p *FloatingIPPlugin) checkAppAndReplicas(keyObj *util.KeyObj) (appExist bool, replicas int32, retErr error) {
	if keyObj.StatefulSet() {
		return p.getStsReplicas(keyObj)
	} else if gvr := p.crdKey.GetGroupVersionResource(keyObj.AppTypePrefix); gvr != nil {
		replica, err := p.crdCache.GetReplicas(*gvr, keyObj.Namespace, keyObj.AppName)
		if err != nil {
			if !metaErrs.IsNotFound(err) {
				retErr = err
			}
			// app not exist
		} else {
			appExist = true
			replicas = int32(replica)
		}
	} else {
		retErr = fmt.Errorf("Unknown app")
	}
	return
}

func (p *FloatingIPPlugin) getStsReplicas(keyObj *util.KeyObj) (appExist bool, replicas int32, retErr error) {
	ss, err := p.StatefulSetLister.StatefulSets(keyObj.Namespace).Get(keyObj.AppName)
	if err != nil {
		if !metaErrs.IsNotFound(err) {
			retErr = err
			return
		}
	} else {
		appExist = true
		replicas = 1
		if ss.Spec.Replicas != nil {
			replicas = *ss.Spec.Replicas
		}
	}
	return
}

func (p *FloatingIPPlugin) shouldRelease(keyObj *util.KeyObj, parentAppExist bool,
	replicas int32) (bool, string, error) {
	if !parentAppExist {
		return true, deletedAndParentAppNotExistPod, nil
	}
	index, err := parsePodIndex(keyObj.KeyInDB)
	if err != nil {
		return false, "", fmt.Errorf("invalid pod name of key %s: %v", keyObj.KeyInDB, err)
	}
	if replicas < int32(index)+1 {
		return true, deletedAndScaledDownAppPod, nil
	}
	return false, "pod index is less than replicas", nil
}
