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
