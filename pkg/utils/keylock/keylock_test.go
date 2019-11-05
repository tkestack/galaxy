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
package keylock

import (
	"sync"
	"testing"
)

//race test
func TestLockUnLock(t *testing.T) {
	l := NewKeylock()
	key := []byte("pod1-22-31")
	a1 := 0
	a2 := 0
	go func() {
		l.Lock(key)
		a1 = 1
		a2 = 1
		l.Unlock(key)
	}()
	l.Lock(key)
	a1 = 2
	a2 = 2
	l.Unlock(key)
	if a1 != a2 {
		t.Fatal()
	}
}

func TestRaceLocking(t *testing.T) {
	l := NewKeylock()
	key := []byte("key")
	var count int
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l.Lock(key)
			count++
			if count > 1 {
				t.Fatal()
			}
			count--
			l.Unlock(key)
		}(i)
	}
	wg.Wait()
}

func TestUnLockUnlock(t *testing.T) {
	l := NewKeylock()
	key := []byte("pod1-22-31")
	assertPanic(t, func() { l.Unlock(key) })
}

func assertPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	f()
}
