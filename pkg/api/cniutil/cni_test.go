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
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
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

func TestGetNetworkConfig(t *testing.T) {
	nc1 := []byte(`{"type": "t1", "name": "n1"}`)
	dir, err := ioutil.TempDir("", "TestGetNetworkConfig")
	if err != nil {
		t.Fatal(err)
	}
	defer func(dir string) {
		os.RemoveAll(dir)
	}(dir)
	if err := ioutil.WriteFile(filepath.Join(dir, "temp.conf"), nc1, 0644); err != nil {
		t.Fatal(err)
	}
	if nc, err := GetNetworkConfig("n1", dir); err != nil || !bytes.Equal(nc1, nc) {
		t.Fatalf("nc %s, err %v", string(nc), err)
	}

	// test config in a sub directory
	dir, err = ioutil.TempDir("", "TestGetNetworkConfig")
	if err != nil {
		t.Fatal(err)
	}
	defer func(dir string) {
		os.RemoveAll(dir)
	}(dir)
	if err := os.Mkdir(filepath.Join(dir, "multus"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "multus", "temp.conf"), nc1, 0644); err != nil {
		t.Fatal(err)
	}
	if nc, err := GetNetworkConfig("n1", dir); err != nil || !bytes.Equal(nc1, nc) {
		t.Fatalf("nc %s, err %v", string(nc), err)
	}
}
