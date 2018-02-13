/*
Copyright 2018 The Kubernetes Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TAppNameKey                  = "tapp_name_key"
	TAppHashKey                  = "tapp_template_hash_key"
	TAppUniqHashKey              = "tapp_uniq_hash_key"
	TAppInstanceKey              = "tapp_instance_key"
	APP_TYPE                     = "gaia_app_type"
	APP_TYPE_TAPP                = "gaia_app_type_tapp"
	TAppRollingUpdateTemplateKey = "tapp.gaia.oa.com/rollingUpdate_template_key"
	TAppRollingUpateMaxUnav      = "tapp.gaia.oa.com/rollingUpdate_max_unavailable"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TApp represents a set of pods with consistent identities.
type TApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired identities of pods in this tapp.
	Spec TAppSpec `json:"spec,omitempty"`

	// Status is the current status of pods in this TApp. This data
	// may be out of date by some window of time.
	Status TAppStatus `json:"status,omitempty"`
}

// A TAppSpec is the specification of a TApp.
type TAppSpec struct {
	// Replicas is the desired number of replicas of the given Template.
	// These are replicas in the sense that they are instantiations of the
	// same Template, but individual replicas also have a consistent identity.
	Replicas int32 `json:"replicas,omitempty"`

	// Selector is a label query over pods that should match the replica count.
	// If empty, defaulted to labels on the pod template.
	// More info: http://releases.k8s.io/release-1.4/docs/user-guide/labels.md#label-selectors
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Template is the object that describes the pod that will be initial created/default scaled
	// it should be added to TemplatePool
	Template corev1.PodTemplateSpec `json:"template"`

	// TemplatePool stores template hash key --> podTemplate
	TemplatePool map[string]corev1.PodTemplateSpec `json:"templatePool,omitempty"`

	// Statuses stores desired instance status instanceID --> desiredStatus
	Statuses map[string]InstanceStatus `json:"statuses,omitempty"`

	// Templates stores instanceID --> template hash key
	Templates map[string]string `json:"templates,omitempty"`
}

type InstanceStatus string

const (
	INSTANCE_NOTCREATED InstanceStatus = "NotCreated"
	INSTANCE_PENDING    InstanceStatus = "Pending"
	INSTANCE_RUNNING    InstanceStatus = "Running"
	INSTANCE_UPATING    InstanceStatus = "Updating"
	INSTANCE_POD_FAILED InstanceStatus = "PodFailed"
	INSTANCE_POD_SUCC   InstanceStatus = "PodSucc"
	INSTANCE_KILLING    InstanceStatus = "Killing"
	INSTANCE_KILLED     InstanceStatus = "Killed"
	INSTANCE_FAILED     InstanceStatus = "Failed"
	INSTANCE_SUCC       InstanceStatus = "Succ"
	INSTANCE_UNKNOWN    InstanceStatus = "Unknown"
)

var InstanceStatusAll []InstanceStatus = []InstanceStatus{INSTANCE_NOTCREATED, INSTANCE_PENDING, INSTANCE_RUNNING, INSTANCE_UPATING, INSTANCE_FAILED, INSTANCE_KILLING, INSTANCE_SUCC, INSTANCE_KILLED}

type AppStatus string

const (
	APP_PENDING AppStatus = "Pending"
	APP_RUNNING AppStatus = "Running"
	APP_FAILED  AppStatus = "Failed"
	APP_SUCC    AppStatus = "Succ"
	APP_KILLED  AppStatus = "Killed"
)

// TAppStatus represents the current state of a TApp.
type TAppStatus struct {
	// most recent generation observed by this autoscaler.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Replicas is the number of actual replicas.
	Replicas int32 `json:"replicas"`

	// AppStatus describe the current TApp state
	AppStatus AppStatus `json:"appStatus,omitempty"`

	// Statues stores actual instanceID --> InstanceStatus
	Statuses map[string]InstanceStatus `json:"statuses,omitempty"`

	// Templates stores actual instanceID --> template hash key
	Templates map[string]string `json:"templates,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TAppList is a collection of TApp.
type TAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TApp `json:"items"`
}
