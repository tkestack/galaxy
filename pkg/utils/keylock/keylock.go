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
	"sync/atomic"
	"time"
)

var (
	KIB31  = 8191   //takes 31KIB memory
	KIB511 = 131071 //takes 511KIB memory
	MIB2   = 524287 //takes 2MIB memory

	sleepTime = 10 * time.Millisecond
)

func NewKeylock() *Keylock {
	return &Keylock{locks: make([]uint32, MIB2), keyGen: Crc32Mod, sleepTime: sleepTime}
}

func New(len uint64, keyGen KeyGen, sleepTime time.Duration) *Keylock {
	return &Keylock{locks: make([]uint32, len), keyGen: keyGen, sleepTime: sleepTime}
}

type Keylock struct {
	locks     []uint32
	keyGen    KeyGen
	sleepTime time.Duration
}

func (l *Keylock) GetLockIndex(key []byte) uint32 {
	return l.keyGen(key, len(l.locks))
}

func (l *Keylock) Lock(key []byte) {
	l.RawLock(l.keyGen(key, len(l.locks)))
}

func (l *Keylock) RawLock(index uint32) {
	for {
		if atomic.CompareAndSwapUint32(&l.locks[index], 0, 1) {
			return
		}
		time.Sleep(sleepTime)
	}
}

func (l *Keylock) Unlock(key []byte) {
	l.RawUnlock(l.keyGen(key, len(l.locks)))
}

func (l *Keylock) RawUnlock(index uint32) {
	if atomic.CompareAndSwapUint32(&l.locks[index], 1, 0) {
		return
	}
	panic("unlock of unlocked bytelock")
}
