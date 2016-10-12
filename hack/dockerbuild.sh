#!/usr/bin/env bash

package=git.code.oa.com/gaiastack/galaxy
docker run --rm -v `pwd`:/go/src/$package -w /go/src/$package golang:1.6.2 bash -c /go/src/$package/hack/build.sh