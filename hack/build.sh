#!/usr/bin/env bash

set -e

bin_prefix="galaxy"
package=git.code.oa.com/gaiastack/galaxy
cni_package=github.com/containernetworking/cni

mkdir -p `dirname /go/src/$cni_package`
ln -s /go/src/$package/vendor/$cni_package /go/src/$cni_package
echo "Building ipam plugins"
echo "   host-local"
# build host-local ipam plugin
go build -o /go/src/$package/bin/${bin_prefix}-ipam-local -v $cni_package/plugins/ipam/host-local
echo "Building plugins"
echo "   loopback"
go build -o /go/src/$package/bin/${bin_prefix}-loopback -v $cni_package/plugins/main/loopback

# build galaxy cni plugins
PLUGINS="/go/src/$package/cni/*"
for d in $PLUGINS; do
	if [ -d $d ]; then
		plugin=$(basename $d)
		echo "  " $plugin
		go build -o /go/src/$package/bin/${bin_prefix}-$plugin -v $package/cni/$plugin
	fi
done