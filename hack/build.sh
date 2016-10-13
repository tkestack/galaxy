#!/usr/bin/env bash

set -e

# we are in the project root dir
cur_dir=`pwd`
bin_prefix="galaxy"
package=git.code.oa.com/gaiastack/galaxy
cni_package=github.com/containernetworking/cni

mkdir -p go/src/`dirname $package`
ln -s $cur_dir $cur_dir/go/src/$package
export GOPATH=$cur_dir/go
mkdir -p `dirname $GOPATH/src/$cni_package`
ln -s $GOPATH/src/$package/vendor/$cni_package $GOPATH/src/$cni_package
function cleanup() {
	rm $GOPATH/src/$cni_package
	rm $cur_dir/go/src/$package
}
trap cleanup EXIT
echo "Building ipam plugins"
echo "   host-local"
# build host-local ipam plugin
go build -o bin/host-local -v $cni_package/plugins/ipam/host-local
echo "Building plugins"
echo "   loopback"
# we can't add prefix to loopback binary cause k8s hard code the type name of lo plugin
go build -o bin/loopback -v $cni_package/plugins/main/loopback
echo "   bridge"
go build -o bin/${bin_prefix}-bridge -v $cni_package/plugins/main/bridge
echo "   flannel"
go build -o bin/${bin_prefix}-flannel -v $cni_package/plugins/meta/flannel

# build galaxy cni plugins
PLUGINS="$GOPATH/src/$package/cni/*"
for d in $PLUGINS; do
	if [ -d $d ]; then
		plugin=$(basename $d)
		echo "  " $plugin
		go build -o bin/${bin_prefix}-$plugin -v $package/cni/$plugin
	fi
done