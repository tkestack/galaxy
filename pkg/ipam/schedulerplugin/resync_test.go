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
	"net"
	"testing"

	. "tkestack.io/galaxy/pkg/ipam/schedulerplugin/testing"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

func TestResyncAppNotExist(t *testing.T) {
	pod1 := CreateDeploymentPod("dp-xxx-yyy", "ns1", poolAnnotation("pool1"))
	pod2 := CreateDeploymentPod("dp2-aaa-bbb", "ns2", immutableAnnotation)
	fipPlugin, stopChan, _ := createPluginTestNodes(t)
	defer func() { stopChan <- struct{}{} }()
	pod1Key, _ := util.FormatKey(pod1)
	pod2Key, _ := util.FormatKey(pod2)

	if err := fipPlugin.ipam.AllocateSpecificIP(pod1Key.KeyInDB, net.ParseIP("10.49.27.205"), parseReleasePolicy(&pod1.ObjectMeta), ""); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.ipam.AllocateSpecificIP(pod2Key.KeyInDB, net.ParseIP("10.49.27.216"), parseReleasePolicy(&pod2.ObjectMeta), ""); err != nil {
		t.Fatal(err)
	}
	if err := fipPlugin.resyncPod(fipPlugin.ipam); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.49.27.205", pod1Key.PoolPrefix()); err != nil {
		t.Fatal(err)
	}
	if err := checkIPKey(fipPlugin.ipam, "10.49.27.216", ""); err != nil {
		t.Fatal(err)
	}
}
