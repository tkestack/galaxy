#!/usr/bin/env bash

set -e

flags=${debug:+"-v"}
ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
PKG=git.code.oa.com/tkestack/galaxy
BIN_PREFIX="galaxy"
CNI_PKG=github.com/containernetworking/plugins

mkdir -p go/src/`dirname ${PKG}`
mkdir -p go/src/`dirname ${CNI_PKG}`
ln -sfn ${ROOT} ${ROOT}/go/src/${PKG}
export GOPATH=${ROOT}/go
export GOOS=linux
if [ ! -d ${ROOT}/go/src/${CNI_PKG} ]; then
	tar zxf ${ROOT}/hack/plugins-0.6.0.tar.gz -C ${ROOT}/go/src/github.com/containernetworking/
	mv ${ROOT}/go/src/github.com/containernetworking/plugins-0.6.0 ${ROOT}/go/src/github.com/containernetworking/plugins
fi
function cleanup() {
	rm ${ROOT}/go/src/${PKG}
}
trap cleanup EXIT
echo "Building tools"
echo "   disable-ipv6"
go build -o bin/disable-ipv6 $flags ${PKG}/cmd/disable-ipv6
echo "Building ipam plugins"
echo "   host-local"
# build host-local ipam plugin
go build -o bin/host-local $flags ${CNI_PKG}/plugins/ipam/host-local
echo "Building plugins"
echo "   loopback"
# we can't add prefix to loopback binary cause k8s hard code the type name of lo plugin
go build -o bin/loopback $flags ${CNI_PKG}/plugins/main/loopback
echo "   bridge"
go build -o bin/${BIN_PREFIX}-bridge $flags ${CNI_PKG}/plugins/main/bridge
echo "   flannel"
go build -o bin/${BIN_PREFIX}-flannel $flags ${CNI_PKG}/plugins/meta/flannel

# build galaxy cni plugins
PLUGINS="$GOPATH/src/${PKG}/cni/k8s-vlan $GOPATH/src/${PKG}/cni/sdn $GOPATH/src/${PKG}/cni/veth $GOPATH/src/${PKG}/cni/k8s-sriov"
for d in $PLUGINS; do
	if [ -d $d ]; then
		plugin=$(basename $d)
		echo "  " $plugin
		go build -o bin/${BIN_PREFIX}-$plugin $flags ${PKG}/cni/$plugin
	fi
done

# build cni plugins (no galaxy prefix)
CNI_PLUGINS="$GOPATH/src/${PKG}/cni/tke-route-eni"
for d in $CNI_PLUGINS; do
	if [ -d $d ]; then
		plugin=$(basename $d)
		echo "  " $plugin
		go build -o bin/$plugin $flags ${PKG}/cni/$plugin
	fi
done

# build galaxy
echo "Building galaxy"
go build -o bin/galaxy $flags $PKG/cmd/galaxy
