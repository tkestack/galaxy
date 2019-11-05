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
/*
Copyright 2017 The Kubernetes Authors.

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

package testing

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"tkestack.io/galaxy/pkg/utils/ipset"
)

const testVersion = "v6.19"

var (
	set = &ipset.IPSet{
		Name:       "foo",
		SetType:    ipset.HashIPPort,
		HashFamily: ipset.ProtocolFamilyIPV4,
	}
)

func TestVersion(t *testing.T) {
	fake := NewFake(testVersion)
	version, err := fake.GetVersion()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if version != testVersion {
		t.Errorf("Unexpected version mismatch, expected: %s, got: %s", testVersion, version)
	}
}

func TestSet(t *testing.T) {
	fake := NewFake(testVersion)
	// create a set
	if err := fake.CreateSet(set, true); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := checkSet(fake, "foo"); err != nil {
		t.Error(err)
	}
	// create another set
	set2 := &ipset.IPSet{
		Name:       "bar",
		SetType:    ipset.HashIPPortIP,
		HashFamily: ipset.ProtocolFamilyIPV6,
	}
	if err := fake.CreateSet(set2, true); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := checkSet(fake, "foo", "bar"); err != nil {
		t.Error(err)
	}
	// Destroy a given set
	if err := fake.DestroySet(set.Name); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if fake.Sets[set.Name] != nil {
		t.Errorf("Unexpected set: %v", fake.Sets[set.Name])
	}

	// Destroy all sets
	if err := fake.DestroyAllSets(); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := checkSet(fake); err != nil {
		t.Error(err)
	}
}

func checkSet(fake *FakeIPSet, names ...string) error {
	setList, err := fake.ListSets()
	if err != nil {
		return fmt.Errorf("Unexpected error: %v", err)
	}
	expect := len(names)
	if len(setList) != expect {
		return fmt.Errorf("Expected %d sets, got %d", expect, len(setList))
	}
	expectedSets := sets.NewString(names...)
	if !expectedSets.Equal(sets.NewString(setList...)) {
		return fmt.Errorf("Unexpected sets mismatch, expected: %v, got: %v", expectedSets, setList)
	}
	return nil
}

func TestEntry(t *testing.T) {
	fake := NewFake(testVersion)
	if err := fake.CreateSet(set, true); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// add two entries
	fake.AddEntry("192.168.1.1,tcp:8080", set, true)
	fake.AddEntry("192.168.1.2,tcp:8081", set, true)
	if err := checkEntries(fake, set, "192.168.1.1,tcp:8080", "192.168.1.2,tcp:8081"); err != nil {
		t.Error(err)
	}
	// delete entry from a given set
	if err := fake.DelEntry("192.168.1.1,tcp:8080", set.Name); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := checkEntries(fake, set, "192.168.1.2,tcp:8081"); err != nil {
		t.Error(err)
	}
	// Flush set
	if err := fake.FlushSet(set.Name); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if err := checkEntries(fake, set); err != nil {
		t.Error(err)
	}
}

func checkEntries(fake *FakeIPSet, set *ipset.IPSet, expected ...string) error {
	entries, err := fake.ListEntries(set.Name)
	if err != nil {
		return fmt.Errorf("Unexpected error: %v", err)
	}
	expectedLen := len(expected)
	if len(entries) != expectedLen {
		return fmt.Errorf("Expected %d entries, got %d", expectedLen, len(entries))
	}
	expectedEntries := sets.NewString(expected...)
	if !expectedEntries.Equal(sets.NewString(entries...)) {
		return fmt.Errorf("Unexpected entries mismatch, expected: %v, got: %v", expectedEntries, entries)
	}
	for _, expect := range expectedEntries.List() {
		// test entries
		found, err := fake.TestEntry(expect, set.Name)
		if err != nil {
			return fmt.Errorf("Unexpected error: %v", err)
		}
		if !found {
			return fmt.Errorf("Unexpected entry %s not found", expect)
		}
	}
	return nil
}

// TODO: Test ignoreExistErr=false
