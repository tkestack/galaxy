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
	"bytes"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"tkestack.io/galaxy/pkg/utils/ipset"
)

// FakeIPSet is a no-op implementation of ipset Interface
type FakeIPSet struct {
	// version of ipset util
	Version string
	// The key of Sets map is the ip set name
	Sets map[string]*ipset.IPSet
	// The key of Entries map is the ip set name where the entries exists
	Entries map[string]sets.String
}

// NewFake create a new fake ipset interface - it initialize the FakeIPSet.
func NewFake(version string) *FakeIPSet {
	return &FakeIPSet{
		Version: version,
		Sets:    make(map[string]*ipset.IPSet),
		Entries: make(map[string]sets.String),
	}
}

// GetVersion is part of interface.
func (f *FakeIPSet) GetVersion() (string, error) {
	return f.Version, nil
}

// FlushSet is part of interface.  It deletes all entries from a named set but keeps the set itself.
func (f *FakeIPSet) FlushSet(set string) error {
	if f.Entries == nil {
		return fmt.Errorf("entries map can't be nil")
	}

	// delete all entry elements
	for true {
		if _, has := f.Entries[set].PopAny(); has {
			continue
		}
		break
	}
	return nil
}

// DestroySet is part of interface.  It deletes both the entries and the set itself.
func (f *FakeIPSet) DestroySet(set string) error {
	delete(f.Sets, set)
	delete(f.Entries, set)
	return nil
}

// DestroyAllSets is part of interface.
func (f *FakeIPSet) DestroyAllSets() error {
	f.Sets = nil
	f.Entries = nil
	return nil
}

// CreateSet is part of interface.
func (f *FakeIPSet) CreateSet(set *ipset.IPSet, ignoreExistErr bool) error {
	if f.Sets[set.Name] != nil {
		if !ignoreExistErr {
			// already exists
			return fmt.Errorf("Set cannot be created: set with the same name already exists")
		}
		return nil
	}
	f.Sets[set.Name] = set
	// initialize entry map
	f.Entries[set.Name] = sets.NewString()
	return nil
}

// AddEntry is part of interface.
func (f *FakeIPSet) AddEntry(entry string, set *ipset.IPSet, ignoreExistErr bool) error {
	if f.Entries[set.Name].Has(entry) {
		if !ignoreExistErr {
			// already exists
			return fmt.Errorf("Element cannot be added to the set: it's already added")
		}
		return nil
	}
	f.Entries[set.Name].Insert(entry)
	return nil
}

// DelEntry is part of interface.
func (f *FakeIPSet) DelEntry(entry string, set string) error {
	if f.Entries == nil {
		return fmt.Errorf("entries map can't be nil")
	}
	f.Entries[set].Delete(entry)
	return nil
}

// TestEntry is part of interface.
func (f *FakeIPSet) TestEntry(entry string, set string) (bool, error) {
	if f.Entries == nil {
		return false, fmt.Errorf("entries map can't be nil")
	}
	found := f.Entries[set].Has(entry)
	return found, nil
}

// ListEntries is part of interface.
func (f *FakeIPSet) ListEntries(set string) ([]string, error) {
	if f.Entries == nil {
		return nil, fmt.Errorf("entries map can't be nil")
	}
	return f.Entries[set].UnsortedList(), nil
}

// ListSets is part of interface.
func (f *FakeIPSet) ListSets() ([]string, error) {
	res := []string{}
	for set := range f.Sets {
		res = append(res, set)
	}
	return res, nil
}

func (f *FakeIPSet) AddEntryWithOptions(entry *ipset.Entry, set *ipset.IPSet, ignoreExistErr bool) error {
	return f.AddEntry(strings.Join(append([]string{entry.String()}, entry.Options...), " "), set, ignoreExistErr)
}

func (f *FakeIPSet) DelEntryWithOptions(set, entry string, options ...string) error {
	return f.DelEntry(strings.Join(append([]string{entry}, options...), " "), set)
}

func (f *FakeIPSet) SaveAllSets() ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	var setNames []string
	for name := range f.Sets {
		setNames = append(setNames, name)
	}
	sort.Strings(setNames)
	for _, setName := range setNames {
		if f.Sets[setName] == nil {
			// should never happen
			continue
		}
		buf.WriteString(fmt.Sprintf("Name: %s\n", setName))
		buf.WriteString(fmt.Sprintf("Type: %s\n", string(f.Sets[setName].SetType)))
		buf.WriteString("Members:\n")
		if entrySet := f.Entries[setName]; entrySet != nil {
			for _, entry := range entrySet.List() {
				buf.WriteString(entry)
				buf.WriteString("\n")
			}
		}
		buf.WriteString("\n")
	}
	if buf.Len() > 0 {
		buf.Truncate(buf.Len() - 1)
	}
	return buf.Bytes(), nil
}

var _ = ipset.Interface(&FakeIPSet{})
