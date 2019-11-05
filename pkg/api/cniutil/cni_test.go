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
package cniutil

import (
	"encoding/json"
	"testing"
)

func TestReverse(t *testing.T) {
	infos := []*NetworkInfo{{NetworkType: "2", Args: map[string]string{"2": "2"}}, {NetworkType: "1"}, {NetworkType: "3"}}
	reverse(infos)
	data, err := json.Marshal(infos)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `[{"NetworkType":"3","Args":null,"Conf":null,"IfName":""},{"NetworkType":"1","Args":null,"Conf":null,"IfName":""},{"NetworkType":"2","Args":{"2":"2"},"Conf":null,"IfName":""}]` {
		t.Fatal(string(data))
	}
}
