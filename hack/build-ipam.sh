#!/usr/bin/env bash

set -e

flags=${debug:+"-v"}
# we are in the project root dir
cur_dir=`pwd`
package=git.code.oa.com/gaiastack/galaxy

mkdir -p go/src/`dirname $package`
ln -sfn $cur_dir $cur_dir/go/src/$package
export GOPATH=$cur_dir/go
export GOOS=linux
function cleanup() {
	rm $cur_dir/go/src/$package
}
trap cleanup EXIT

# build galaxy-ipam
echo "Building galaxy-ipam"
go build -o bin/galaxy-ipam $flags $package/cmd/galaxy-ipam
