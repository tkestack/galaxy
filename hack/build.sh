#!/usr/bin/env bash

set -e

GOBUILD_FLAGS=${debug:+"-v"}
ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
source ${ROOT}/hack/init.sh
BINDIR=bin
CNI_BIN=https://github.com/containernetworking/plugins/releases/download/v0.6.0/cni-plugins-amd64-v0.6.0.tgz

function build::get_basic_cni() {
  local BIN_PREFIX="galaxy"

  CNI_TGZ=hack/cni.tgz
  if [ ! -f $CNI_TGZ ]; then
    echo "Downloading $CNI_BIN"
    curl -L $CNI_BIN -o $CNI_TGZ
  fi
  mkdir -p $BINDIR
  tar zxf $CNI_TGZ -C $BINDIR
  # TODO remove these renames
  mv $BINDIR/bridge $BINDIR/$BIN_PREFIX-bridge
  mv $BINDIR/flannel $BINDIR/$BIN_PREFIX-flannel
}

function build::galaxy() {
  local BIN_PREFIX="galaxy"

  echo "Building tools"
  echo "   disable-ipv6"
  go build -o $BINDIR/disable-ipv6 $GOBUILD_FLAGS ${PKG}/cmd/disable-ipv6
  echo "Building plugins"

  # build galaxy cni plugins
  PLUGINS="${PKG}/cni/k8s-vlan ${PKG}/cni/sdn ${PKG}/cni/veth ${PKG}/cni/k8s-sriov"
  for d in $PLUGINS; do
    plugin=$(basename $d)
    echo "  " $plugin
    go build -o $BINDIR/${BIN_PREFIX}-$plugin $GOBUILD_FLAGS ${PKG}/cni/$plugin
  done

  # build cni plugins (no galaxy prefix)
  CNI_PLUGINS="${PKG}/cni/tke-route-eni"
  for d in $CNI_PLUGINS; do
    plugin=$(basename $d)
    echo "  " $plugin
    go build -o $BINDIR/$plugin $GOBUILD_FLAGS ${PKG}/cni/$plugin
  done

  # build galaxy
  echo "Building galaxy"
  echo go build -o $BINDIR/galaxy $GOBUILD_FLAGS -ldflags "$(init::print_ldflags)" $PKG/cmd/galaxy
  go build -o $BINDIR/galaxy $GOBUILD_FLAGS -ldflags "$(init::print_ldflags)" $PKG/cmd/galaxy
}

function build::ipam() {
  echo "Building galaxy-ipam"
  echo "   galaxy-ipam"
  echo go build -o $BINDIR/galaxy-ipam $GOBUILD_FLAGS -ldflags "$(init::print_ldflags)" ${PKG}/cmd/galaxy-ipam
  go build -o $BINDIR/galaxy-ipam $GOBUILD_FLAGS -ldflags "$(init::print_ldflags)" ${PKG}/cmd/galaxy-ipam
}

function build::galaxy_image() {
  echo "Building galaxy image..."
  mkdir -p ${ROOT}/bin/images
  local temp_dockerfile=$ROOT/bin/galaxy.dockerfile
  cat > $temp_dockerfile << EOF
FROM centos:7
MAINTAINER louis <louisssgong@tencent.com>
LABEL version="${VERSION}"
LABEL description="This Dockerfile is written for galaxy"
WORKDIR /root/
RUN yum install -y iproute iptables
COPY bin/galaxy /usr/bin/
COPY bin/disable-ipv6 bin/galaxy-k8s-sriov bin/galaxy-k8s-vlan bin/galaxy-veth bin/galaxy-bridge bin/galaxy-flannel bin/host-local bin/loopback bin/tke-route-eni bin/galaxy-sdn /opt/cni/galaxy/bin/
EOF
  local image=$REGISTRY/galaxy:${VERSION}
  echo docker build -f $temp_dockerfile -t $image .
  docker build -f $temp_dockerfile -t $image .
  docker push $image
}

function build::ipam_image() {
  echo "Building galaxy-ipam image..."
  mkdir -p ${ROOT}/bin/images
  local temp_dockerfile=$ROOT/bin/galaxy_ipam.dockerfile
  cat > $temp_dockerfile << EOF
FROM centos:7
MAINTAINER louis <louisssgong@tencent.com>
LABEL version="${VERSION}"
LABEL description="This Dockerfile is written for galaxy"
WORKDIR /root/
COPY bin/galaxy-ipam /usr/bin/
EOF
  local image=$REGISTRY/galaxy-ipam:${VERSION}
  echo docker build -f $temp_dockerfile -t $image .
  docker build -f $temp_dockerfile -t $image .
  docker push $image
}

function build::verify() {
  bad_files=$(gofmt -s -l cni/ pkg/ cmd/ tools/)
  if [[ -n "${bad_files}" ]]; then
    echo "gofmt -s -w' needs to be run on the following files: "
    echo "${bad_files}"
    exit 1
  fi
}

(
  for arg; do
    case $arg in
    binary_galaxy)
      build::get_basic_cni
      if ! init::docker_builded "$arg"; then
        build::galaxy
      fi
      ;;
    binary_ipam)
      if ! init::docker_builded "$arg"; then
        build::ipam
      fi
      ;;
    image_galaxy)
      build::galaxy_image
      ;;
    image_ipam)
      build::ipam_image
      ;;
    verify)
      build::verify
      ;;
    *)
      echo unknown arg $arg
    esac
  done
)
