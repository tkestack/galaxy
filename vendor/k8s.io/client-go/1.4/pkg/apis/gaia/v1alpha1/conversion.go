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
	"fmt"
	"k8s.io/client-go/1.4/pkg/api"
	_ "k8s.io/client-go/1.4/pkg/api/unversioned"
	_ "k8s.io/client-go/1.4/pkg/apis/gaia"
	_ "k8s.io/client-go/1.4/pkg/conversion"
	"k8s.io/client-go/1.4/pkg/runtime"
)

func addConversionFuncs(scheme *runtime.Scheme) error {
	return api.Scheme.AddFieldLabelConversionFunc("gaia/v1alpha1", "TApp",
		func(label, value string) (string, string, error) {
			switch label {
			case "metadata.name", "metadata.namespace", "status.successful":
				return label, value, nil
			default:
				return "", "", fmt.Errorf("field label not supported: %s", label)
			}
		},
	)
}
