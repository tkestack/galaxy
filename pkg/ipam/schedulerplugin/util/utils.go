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
package util

import (
	"fmt"
	"strings"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
)

// resolveDeploymentName return deployment name if pod is created by deployment
func resolveDeploymentName(pod *corev1.Pod) string {
	if len(pod.OwnerReferences) == 1 && pod.OwnerReferences[0].Kind == "ReplicaSet" {
		// assume pod belong to deployment for convenient
		// can't parse deployment from pod name directly if deployment name is nearly 63 bytes
		// e.g. if deployment name is dp1234567890dp1234567890dp1234567890dp1234567890dp1234567890dp1 (63 bytes)
		// ReplicaSet name is dp1234567890dp1234567890dp1234567890dp1234567890dp1234567890dp1-69fd8dbc5c (74 bytes)
		// pod name is generated like dp1234567890dp1234567890dp1234567890dp1234567890dp1234567848p74
		ownerName := pod.OwnerReferences[0].Name
		lastIndex := strings.LastIndex(ownerName, "-")
		if lastIndex == -1 {
			// parent app is replicatsets with a name that doesn't include '-' instead of deployment
			return ownerName
		}
		// TODO for replicasets pods, this should return full ownerName. When we change this, be sure to fix release fip API too
		return ownerName[:lastIndex]
	}
	return ""
}

type KeyObj struct {
	// stores the key format in IPAM
	// for deployment dp_namespace_deploymentName_podName,
	// for pool pool__poolName_dp_namespace_deploymentName_podName, for statefulset
	// sts_namespace_statefulsetName_podName
	// If deployment name is 63 bytes, e.g. dp1234567890dp1234567890dp1234567890dp1234567890dp1234567890dp1
	// deployment pod name will be 63 bytes with modified suffix, e.g.
	// dp1234567890dp1234567890dp1234567890dp1234567890dp1234567848p74
	// So we can't get deployment name from pod name and have to store deployment name with pod name
	KeyInDB       string
	AppName       string
	AppTypePrefix string
	PodName       string
	Namespace     string
	// the annotation value if pod has pool annotation
	PoolName string
}

func NewKeyObj(appTypePrefix string, namespace, appName, podName, poolName string) *KeyObj {
	k := &KeyObj{AppTypePrefix: appTypePrefix, AppName: appName, PodName: podName, Namespace: namespace,
		PoolName: poolName}
	k.genKey()
	return k
}

func (k *KeyObj) Deployment() bool {
	return k.AppTypePrefix == DeploymentPrefixKey
}

func (k *KeyObj) StatefulSet() bool {
	return k.AppTypePrefix == StatefulsetPrefixKey
}

func (k *KeyObj) TApp() bool {
	return k.AppTypePrefix == TAppPrefixKey
}

func (k *KeyObj) genKey() {
	var prefix string
	if k.PoolName != "" {
		prefix = fmt.Sprintf("%s%s_", poolPrefix, k.PoolName)
		if k.AppName == "" {
			k.KeyInDB = prefix
			return
		}
	}
	if k.PoolName == "" && k.AppName == "" && k.Namespace == "" {
		k.KeyInDB = ""
		return
	}
	k.KeyInDB = fmt.Sprintf("%s%s%s_%s_%s", prefix, k.AppTypePrefix, k.Namespace, k.AppName, k.PodName)
}

// PoolPrefix returns the common key prefix in IPAM, for deployment dp_namespace_deploymentName_
// for pool pool__poolName_, for statefulset sts_namespace_statefulsetName_
// For now, if it is a statefulset pod, PoolPrefix is useless since we reserve ip by full pod name
// PoolPrefix is used by pool and deployment only.
func (k *KeyObj) PoolPrefix() string {
	if k.PoolName != "" {
		return fmt.Sprintf("%s%s_", poolPrefix, k.PoolName)
	}
	return fmt.Sprintf("%s%s_%s_", k.AppTypePrefix, k.Namespace, k.AppName)
}

func (k *KeyObj) PoolAppPrefix() string {
	if k.PoolName != "" {
		return fmt.Sprintf("%s%s_%s%s_%s_", poolPrefix, k.PoolName, k.AppTypePrefix, k.Namespace, k.AppName)
	}
	return k.PoolPrefix()
}

const (
	// ip pool may be shared with other namespaces, so leave namespace empty
	poolPrefix           = "pool__"
	DeploymentPrefixKey  = "dp_"
	StatefulsetPrefixKey = "sts_"
	TAppPrefixKey        = "tapp_"
)

func FormatKey(pod *corev1.Pod) (*KeyObj, error) {
	pool := constant.GetPool(pod.Annotations)
	keyObj := &KeyObj{
		PoolName:  pool,
		PodName:   pod.Name,
		Namespace: pod.Namespace}
	if len(pod.OwnerReferences) == 0 {
		return keyObj, fmt.Errorf("doesn't support pods which does not have parent app")
	}
	if pod.OwnerReferences[0].Kind == "StatefulSet" {
		keyObj.AppName = pod.OwnerReferences[0].Name
		keyObj.AppTypePrefix = StatefulsetPrefixKey
	} else if pod.OwnerReferences[0].Kind == "TApp" {
		keyObj.AppName = pod.OwnerReferences[0].Name
		keyObj.AppTypePrefix = TAppPrefixKey
	} else {
		deploymentName := resolveDeploymentName(pod)
		if deploymentName == "" {
			return keyObj, fmt.Errorf("unsupported app type")
		}
		keyObj.AppName = deploymentName
		// treat rs like deployment, share the same appPrefixKey
		keyObj.AppTypePrefix = DeploymentPrefixKey
	}
	keyObj.genKey()
	return keyObj, nil
}

func ParseKey(key string) *KeyObj {
	//tapp_ns1_demo_demo-1
	keyObj := &KeyObj{KeyInDB: key}
	removedPoolKey := key
	if strings.HasPrefix(key, poolPrefix) {
		// pool__poolName_deployment_namespace_deploymentName_podName
		// poolName and deployment_namespace_deploymentName_podName
		parts := strings.SplitN(key[len(poolPrefix):], "_", 2)
		if len(parts) != 2 {
			return keyObj
		}
		keyObj.PoolName = parts[0]
		removedPoolKey = parts[1]
	}
	if strings.HasPrefix(removedPoolKey, DeploymentPrefixKey) {
		keyObj.AppTypePrefix = DeploymentPrefixKey
	} else if strings.HasPrefix(removedPoolKey, StatefulsetPrefixKey) {
		keyObj.AppTypePrefix = StatefulsetPrefixKey
	} else if strings.HasPrefix(removedPoolKey, TAppPrefixKey) {
		keyObj.AppTypePrefix = TAppPrefixKey
	}
	keyObj.AppName, keyObj.PodName, keyObj.Namespace = resolvePodKey(removedPoolKey)
	return keyObj
}

// resolvePodKey returns appname, podName, namespace
// "sts_kube-system_fip-bj_fip-bj-111": {"fip-bj", "fip-bj-111", "kube-system"}
func resolvePodKey(key string) (string, string, string) {
	// _ is not a valid char in appname
	parts := strings.Split(key, "_")
	if len(parts) == 4 {
		return parts[2], parts[3], parts[1]
	}
	return "", "", ""
}

func Join(name, namespace string) string {
	return fmt.Sprintf("%s_%s", namespace, name)
}

func PodName(pod *corev1.Pod) string {
	return fmt.Sprintf("%s_%s", pod.Namespace, pod.Name)
}

func StatefulsetName(ss *appv1.StatefulSet) string {
	return fmt.Sprintf("%s_%s", ss.Namespace, ss.Name)
}

func DeploymentName(dp *appv1.Deployment) string {
	return fmt.Sprintf("%s_%s", dp.Namespace, dp.Name)
}
