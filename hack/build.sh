#!/usr/bin/env bash

set -e

bin_prefix="galaxy"
package=git.code.oa.com/gaiastack/galaxy
cni_package=github.com/containernetworking/cni
go_build_vlan="-o /go/src/$package/bin/${bin_prefix}-vlan -v $package/cni/vlan"
go_build_hostlocal="-o /go/src/$package/bin/${bin_prefix}-ipam-local -v $cni_package/plugins/ipam/host-local"
go build $go_build_vlan
mkdir -p `dirname /go/src/$cni_package`
ln -s /go/src/$package/vendor/$cni_package /go/src/$cni_package
go build $go_build_hostlocal