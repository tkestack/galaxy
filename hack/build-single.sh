#!/usr/bin/env bash

set -e

# docker run --rm -v `pwd`:/go/src/$package -w /go/src/$package -e BINARY=cmd/galaxy golang:1.11.4 bash -c /go/src/$package/hack/build-single.sh
# we are in the project root dir
cur_dir=`pwd`
bin_prefix="galaxy"
package=git.code.oa.com/gaiastack/galaxy

mkdir -p go/src/`dirname $package`
ln -sfn $cur_dir $cur_dir/go/src/$package
export GOPATH=$cur_dir/go
function cleanup() {
	rm $cur_dir/go/src/$package
}
trap cleanup EXIT

BINARY_NAME=`echo ${BINARY} | awk -F/ '{print $2}'`
go build -o bin/${BINARY_NAME} -v $package/$BINARY
