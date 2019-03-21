#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)

function build_binary() {
  package=git.code.oa.com/gaiastack/galaxy
  docker run --rm -v `pwd`:/go/src/$package -w /go/src/$package golang:1.11.4 bash -c /go/src/$package/hack/build-ipam.sh
}

function build_ipam_image() {
  VERSION=1.0.0-alpha
  cat > "bin/images/galaxy_ipam.dockerfile" << EOF
FROM centos:7.2.1511
MAINTAINER louis <louisssgong@tencent.com>
LABEL version="${VERSION}"
LABEL description="This Dockerfile is written for galaxy"
WORKDIR /root/
COPY bin/galaxy-ipam /usr/bin/
COPY hack/start-ipam.sh /root/
CMD ["/root/start-ipam.sh"]
EOF
  docker build -f bin/images/galaxy_ipam.dockerfile -t galaxy_ipam:${VERSION} .
  docker tag galaxy_ipam:${VERSION} ccr.ccs.tencentyun.com/tkeimages/galaxy-ipam:${VERSION} .
}

echo "begin to build galaxy-ipam"
build_binary

echo "begin to build image"
mkdir -p ${ROOT}/bin/images
build_ipam_image