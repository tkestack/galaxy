package main

import (
	"flag"
	"syscall"
	"time"

	log "github.com/golang/glog"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

var flagDevice = flag.String("device", "", "device name to listen")

func main() {
	flag.Parse()
	if *flagDevice == "" {
		log.Fatalf("please specify device name")
	}
	link, err := netlink.LinkByName(*flagDevice)
	if err != nil {
		log.Fatalf("failed to get device %s: %v", *flagDevice, err)
	}
	dev := device{l: link}
	dev.MonitorMisses()
}

type device struct {
	l netlink.Link
}

func (dev *device) MonitorMisses() {
	nlsock, err := nl.Subscribe(syscall.NETLINK_ROUTE, syscall.RTNLGRP_NEIGH)
	if err != nil {
		log.Error("Failed to subscribe to netlink RTNLGRP_NEIGH messages")
		return
	}

	for {
		msgs, err := nlsock.Receive()
		if err != nil {
			log.Errorf("Failed to receive from netlink: %v ", err)

			time.Sleep(1 * time.Second)
			continue
		}

		for _, msg := range msgs {
			dev.processNeighMsg(msg)
		}
	}
}

func (dev *device) processNeighMsg(msg syscall.NetlinkMessage) {
	neigh, err := netlink.NeighDeserialize(msg.Data)
	if err != nil {
		log.Error("Failed to deserialize netlink ndmsg: %v", err)
		return
	}

	if int(neigh.LinkIndex) != dev.l.Attrs().Index {
		log.Infof("ignore neigh msg from kernel %v: not equal device id %d", neigh, dev.l.Attrs().Index)
		return
	}

	if msg.Header.Type != syscall.RTM_GETNEIGH && msg.Header.Type != syscall.RTM_NEWNEIGH {
		log.Infof("ignore neigh msg from kernel %v: msg type is wrong %d", neigh, msg.Header.Type)
		return
	}

	log.Infof("receive good neigh msg from kernel %v", neigh)
}
