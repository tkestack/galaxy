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
	"fmt"

	tappv1alpha1 "git.code.oa.com/gaia/tapp-controller/pkg/apis/tappcontroller/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// TAppListerExpansion allows custom methods to be added to
// TAppLister.
type TAppListerExpansion interface {
	GetPodTApps(pod *corev1.Pod) ([]*tappv1alpha1.TApp, error)
}

// TAppNamespaceListerExpansion allows custom methods to be added to
// TAppNamespaceLister.
type TAppNamespaceListerExpansion interface{}

// GetPodTApps returns a list of TApps that potentially match a pod.
// Only the one specified in the Pod's ControllerRef will actually manage it.
// Returns an error only if no matching TApps are found.
func (s *tAppLister) GetPodTApps(pod *corev1.Pod) ([]*tappv1alpha1.TApp, error) {
	var selector labels.Selector

	if len(pod.Labels) == 0 {
		return nil, fmt.Errorf("no TApps found for pod %v because it has no labels", pod.Name)
	}

	list, err := s.TApps(pod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var psList []*tappv1alpha1.TApp
	for _, ps := range list {
		if ps.Namespace != pod.Namespace {
			continue
		}
		selector, err = metav1.LabelSelectorAsSelector(ps.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector: %v", err)
		}

		// If a StatefulSet with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		psList = append(psList, ps)
	}

	if len(psList) == 0 {
		return nil, fmt.Errorf("could not find StatefulSet for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}

	return psList, nil
}
