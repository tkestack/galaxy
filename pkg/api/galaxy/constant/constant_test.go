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
package constant

import (
	"net"
	"reflect"
	"testing"

	"tkestack.io/galaxy/pkg/utils/nets"
)

func TestFormatParseIPInfo(t *testing.T) {
	testCase := []IPInfo{
		{
			IP:      nets.NetsIPNet(&net.IPNet{IP: net.ParseIP("192.168.0.2"), Mask: net.IPv4Mask(255, 255, 0, 0)}),
			Vlan:    2,
			Gateway: net.ParseIP("192.168.0.1"),
		},
		{
			IP:      nets.NetsIPNet(&net.IPNet{IP: net.ParseIP("192.168.0.3"), Mask: net.IPv4Mask(255, 255, 0, 0)}),
			Vlan:    3,
			Gateway: net.ParseIP("192.168.0.1"),
		},
	}
	str, err := FormatIPInfo(testCase)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseIPInfo(str)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed, testCase) {
		t.Fatalf("real: %v, expect: %v", parsed, testCase)
	}
}
