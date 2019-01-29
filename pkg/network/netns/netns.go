package netns

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/golang/glog"
	"github.com/vishvananda/netns"
)

// nolint: errcheck
func NsInvoke(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Save the current network namespace
	origns, err := netns.Get()
	if err != nil {
		glog.Fatal(err)
	}
	defer origns.Close()

	// Create a new network namespace
	newns, err := netns.New()
	if err != nil {
		glog.Fatal(err)
	}
	err = netns.Set(newns)
	if err != nil {
		glog.Fatal(err)
	}
	defer newns.Close()
	f()
	netns.Set(origns)
}

// nolint: errcheck
func InvokeIn(nsFile string, f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Save the current network namespace
	origns, err := netns.Get()
	if err != nil {
		glog.Fatal(err)
	}
	defer origns.Close()

	// Create a new network namespace
	newns, err := netns.GetFromPath(nsFile)
	if err != nil {
		glog.Fatal(err)
	}
	err = netns.Set(newns)
	if err != nil {
		glog.Fatal(err)
	}
	defer newns.Close()
	f()
	netns.Set(origns)
}

// nolint: errcheck
func NewContainerForTest() func() {
	runtime.LockOSThread()
	originmnt, err := GetMntNS()
	if err != nil {
		glog.Fatal(err)
	}
	originnet, err := netns.Get()
	if err != nil {
		glog.Fatal(err)
	}
	if err := syscall.Unshare(syscall.CLONE_NEWNS | syscall.CLONE_NEWNET); err != nil {
		glog.Fatal(err)
	}
	newmnt, err := GetMntNS()
	if err != nil {
		glog.Fatal(err)
	}
	newnet, err := netns.Get()
	if err != nil {
		glog.Fatal(err)
	}
	closables := []closable{&newnet, &newmnt, &originnet, &originmnt}
	return func() {
		SetMntNS(originmnt)
		netns.Set(originnet)
		for i := range closables {
			closables[i].Close()
		}
		runtime.UnlockOSThread()
	}
}

type closable interface {
	Close() error
}

// Get gets a handle to the current threads mount namespace.
func GetMntNS() (netns.NsHandle, error) {
	return netns.GetFromPath(fmt.Sprintf("/proc/%d/task/%d/ns/mnt", os.Getpid(), syscall.Gettid()))
}

// Set sets the current network namespace to the namespace represented
// by NsHandle.
func SetMntNS(ns netns.NsHandle) (err error) {
	return netns.Setns(ns, syscall.CLONE_NEWNS)
}
