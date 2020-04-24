#!/usr/bin/env bash

set -e

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
CURENTDIR=$(pwd)
function cleanup() {
	cd $CURENTDIR
}
trap cleanup EXIT
cd $ROOT

PKG=tkestack.io/galaxy
BIN_PREFIX="galaxy"
# build galaxy cni plugins
PLUGINS="$GOPATH/src/${PKG}/cni/k8s-vlan $GOPATH/src/${PKG}/cni/sdn $GOPATH/src/${PKG}/cni/veth $GOPATH/src/${PKG}/cni/k8s-sriov $GOPATH/src/${PKG}/cni/underlay/veth"
for d in $PLUGINS; do
	if [ -d $d ]; then
	    plugin_dir=${d#"$GOPATH/src/${PKG}/cni/"}
		plugin=${plugin_dir//"/"/"-"}
		echo "  " $plugin
		go build -o bin/${BIN_PREFIX}-$plugin $flags ${PKG}/cni/$plugin
	fi
done
# build galaxy
echo "Building galaxy"
echo go build -o bin/galaxy $flags $PKG/cmd/galaxy
go build -o bin/galaxy $flags $PKG/cmd/galaxy

echo "Building galaxy-ipam"
echo go build -o bin/galaxy-ipam $flags $PKG/cmd/galaxy-ipam
go build -o bin/galaxy-ipam $flags $PKG/cmd/galaxy-ipam
