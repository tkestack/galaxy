/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
)

// +genclient
// +genclient:noStatus
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FloatingIP provides configuration for floatingIP.
type FloatingIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired identities of FloatingIP.
	Spec FloatingIPSpec `json:"spec"`
}

// FloatingIPSpec is spec of FloatingIP.
type FloatingIPSpec struct {
	//key can be resolved as pool, namespace name, app name, app type and pod name
	Key string `json:"key"`
	//attribute used as node ip
	Attribute string `json:"attribute"`
	//policy used as
	Policy constant.ReleasePolicy `json:"policy"`
	//subnet used as node's subnet
	Subnet string `json:"subnet"`
	//FloatingIP update(allocate, release or update) timestamp
	UpdateTime metav1.Time `json:"updateTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FloatingIPList is list of FloatingIP.
type FloatingIPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []FloatingIP `json:"items"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Pool provides configuration for FloatingIP pool which used to store FloatingIP.
type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// The pool size
	Size int `json:"size"`
	// Pre-allocate IP when creating pool
	PreAllocateIP bool `json:"preAllocateIP"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PoolList is list of Pool struct.
type PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Pool `json:"items"`
}
