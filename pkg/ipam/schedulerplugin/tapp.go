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
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

func (p *FloatingIPPlugin) getTAppReplicas(keyObj *util.KeyObj) (appExist bool, replicas int32, retErr error) {
	tapp, err := p.TAppLister.TApps(keyObj.Namespace).Get(keyObj.AppName)
	if err != nil {
		if !metaErrs.IsNotFound(err) {
			retErr = err
			return
		}
	} else {
		appExist = true
		replicas = tapp.Spec.Replicas
	}
	return
}
