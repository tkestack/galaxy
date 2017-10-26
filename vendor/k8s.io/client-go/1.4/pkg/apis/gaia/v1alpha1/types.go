/*
Copyright 2016 The Kubernetes Authors.

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
	"k8s.io/client-go/1.4/pkg/api/unversioned"
	"k8s.io/client-go/1.4/pkg/api/v1"
)

const (
	TAppNameKey     = "tapp_name_key"
	TAppHashKey     = "tapp_template_hash_key"
	TAppUniqHashKey = "tapp_uniq_hash_key"
	TAppInstanceKey = "tapp_instance_key"
)

// +genclient=true

// TApp represents a set of pods with consistent identities.
type TApp struct {
	unversioned.TypeMeta `json:",inline"`
	v1.ObjectMeta        `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the desired identities of pods in this tapp.
	Spec TAppSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Status is the current status of pods in this TApp. This data
	// may be out of date by some window of time.
	Status TAppStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// A TAppSpec is the specification of a TApp.
type TAppSpec struct {
	// Replicas is the desired number of replicas of the given Template.
	// These are replicas in the sense that they are instantiations of the
	// same Template, but individual replicas also have a consistent identity.
	Replicas int32 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`

	// Selector is a label query over pods that should match the replica count.
	// If empty, defaulted to labels on the pod template.
	// More info: http://releases.k8s.io/release-1.4/docs/user-guide/labels.md#label-selectors
	Selector *unversioned.LabelSelector `json:"selector,omitempty" protobuf:"bytes,2,opt,name=selector"`

	// Template is the object that describes the pod that will be initial created/default scaled
	// it should be added to TemplatePool
	Template v1.PodTemplateSpec `json:"template" protobuf:"bytes,3,opt,name=template"`

	// TemplatePool stores template hash key --> podTemplate
	TemplatePool map[string]v1.PodTemplateSpec `json:"templatePool,omitempty" protobuf:"bytes,4,rep,name=templatePool"`

	// Statuses stores desired instance status instanceID --> desiredStatus
	Statuses map[string]InstanceStatus `json:"statuses,omitempty" protobuf:"bytes,5,rep,name=statuses,castvalue=InstanceStatus"`

	// Templates stores instanceID --> template hash key
	Templates map[string]string `json:"templates,omitempty" protobuf:"bytes,6,rep,name=templates"`
}

type InstanceStatus string

const (
	INSTANCE_NOTCREATED InstanceStatus = "notCreated"
	INSTANCE_PENDING    InstanceStatus = "pending"
	INSTANCE_RUNNING    InstanceStatus = "running"
	INSTANCE_POD_FAILED InstanceStatus = "PodFailed"
	INSTANCE_POD_SUCC   InstanceStatus = "PodSucc"
	INSTANCE_KILLING    InstanceStatus = "killing"
	INSTANCE_KILLED     InstanceStatus = "killed"
	INSTANCE_FAILED     InstanceStatus = "failed"
	INSTANCE_SUCC       InstanceStatus = "succ"
	INSTANCE_UNKNOWN    InstanceStatus = "unknown"
)

type AppStatus string

const (
	APP_PENDING AppStatus = "pending"
	APP_RUNNING AppStatus = "running"
	APP_FAILED  AppStatus = "failed"
	APP_SUCC    AppStatus = "succ"
	APP_KILLED  AppStatus = "killed"
)

// TAppStatus represents the current state of a TApp.
type TAppStatus struct {
	// most recent generation observed by this autoscaler.
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// Replicas is the number of actual replicas.
	Replicas int32 `json:"replicas" protobuf:"varint,2,opt,name=replicas"`

	// AppStatus describe the current TApp state
	AppStatus AppStatus `json:"appStatus,omitempty" protobuf:"bytes,3,opt,name=appStatus,casttype=AppStatus"`

	// Statues stores actual instanceID --> InstanceStatus
	Statuses map[string]InstanceStatus `json:"statuses,omitempty" protobuf:"bytes,4,rep,name=statuses,castvalue=InstanceStatus"`

	// Templates stores actual instanceID --> template hash key
	Templates map[string]string `json:"templates,omitempty" protobuf:"bytes,5,rep,name=templates"`
}

// TAppList is a collection of TApp.
type TAppList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Items                []TApp `json:"items" protobuf:"bytes,2,rep,name=items"`
}
