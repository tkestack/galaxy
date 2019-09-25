#!/usr/bin/env bash

package=git.code.oa.com/tkestack/galaxy
docker run --rm -v `pwd`:/go/src/$package -w /go/src/$package golang:1.11.4 go build -o /go/src/$package/bin/netlink_monitor -v $package/tools/netlink_monitor
