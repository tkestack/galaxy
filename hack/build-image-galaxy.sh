#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)

function build_binary() {
  package=git.code.oa.com/gaiastack/galaxy
  docker run --rm -v `pwd`:/go/src/$package -w /go/src/$package golang:1.11.4 bash -c /go/src/$package/hack/build.sh
}

function build_galaxy_image() {
  VERSION=1.0.0-alpha
  cat > "bin/images/galaxy.dockerfile" << EOF
FROM docker.oa.com:8080/public/centos-7.2:latest
MAINTAINER louis <louisssgong@tencent.com>
LABEL version="${VERSION}"
LABEL description="This Dockerfile is written for galaxy"
WORKDIR /root/
RUN yum install -y iproute iproute-doc iptables
COPY bin/galaxy /usr/bin/
COPY bin/disable-ipv6 bin/galaxy-bridge bin/galaxy-flannel bin/galaxy-k8s-sriov bin/galaxy-k8s-vlan bin/galaxy-veth bin/host-local bin/loopback /opt/cni/bin/
COPY hack/start.sh /root/
ENTRYPOINT ["sh", "/root/start.sh"]
EOF
  docker build -f bin/images/galaxy.dockerfile -t docker.oa.com:8080/library/galaxy:${VERSION} .
  docker save -o ${ROOT}/bin/images/galaxy_image_cni.tar.gz docker.oa.com:8080/library/galaxy:${VERSION}
}

echo "begin to build galaxy & cni"
#build_binary

echo "begin to build image"
mkdir -p ${ROOT}/bin/images
build_galaxy_image