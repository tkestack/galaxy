#!/usr/bin/env bash

# Tencent is pleased to support the open source community by making TKEStack
# available.
#
# Copyright (C) 2012-2019 Tencent. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use
# this file except in compliance with the License. You may obtain a copy of the
# License at
#
# https://opensource.org/licenses/Apache-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OF ANY KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations under the License.

PKG=${PKG:-"tkestack.io/galaxy"}
GOBUILD_FLAGS=${debug:+"-v"}
GO_LDFLAGS=${GO_LDFLAGS:-""}

ARCH=${ARCH:-"amd64"}
ROOT_DIR=${ROOT_DIR:-"$(cd $(dirname ${BASH_SOURCE})/../.. && pwd -P)"} 
OUTPUT_DIR=${OUTPUT_DIR:-"${ROOT_DIR}/_output"}
BIN_DIR=${OUTPUT_DIR}/bin-${ARCH}
mkdir -p ${BIN_DIR}

CNI_VERSION="v0.6.0"
CNI_BIN=https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-${ARCH}-${CNI_VERSION}.tgz

function build::get_basic_cni() {
  local BIN_PREFIX="galaxy"

  CNI_TGZ=${OUTPUT_DIR}/cni-${ARCH}.tgz
  if [ ! -f ${CNI_TGZ} ]; then
    echo "Downloading ${CNI_BIN}"
    curl -L ${CNI_BIN} -o ${CNI_TGZ}
  fi
  tar zxf ${CNI_TGZ} -C ${BIN_DIR}
  # TODO remove these renames
  mv ${BIN_DIR}/bridge ${BIN_DIR}/${BIN_PREFIX}-bridge
  mv ${BIN_DIR}/flannel ${BIN_DIR}/${BIN_PREFIX}-flannel
}

function build::galaxy() {
  local BIN_PREFIX="galaxy"

  echo "Building tools"
  echo "   disable-ipv6"
  go build -o ${BIN_DIR}/disable-ipv6 ${GOBUILD_FLAGS} ${PKG}/cmd/disable-ipv6
  echo "Building plugins"

  # build galaxy cni plugins
  PLUGINS="${PKG}/cni/k8s-vlan ${PKG}/cni/sdn ${PKG}/cni/veth ${PKG}/cni/k8s-sriov"
  for d in ${PLUGINS}; do
    plugin=$(basename $d)
    echo "  " ${plugin}
    go build -o ${BIN_DIR}/${BIN_PREFIX}-${plugin} ${GOBUILD_FLAGS} ${PKG}/cni/${plugin}
  done

  # build cni plugins (no galaxy prefix)
  CNI_PLUGINS="${PKG}/cni/tke-route-eni"
  for d in ${CNI_PLUGINS}; do
    plugin=$(basename $d)
    echo "  " ${plugin}
    go build -o ${BIN_DIR}/${plugin} ${GOBUILD_FLAGS} ${PKG}/cni/${plugin}
  done

  # build galaxy
  echo "Building galaxy"
  echo "   galaxy"
  echo go build -o ${BIN_DIR}/galaxy ${GOBUILD_FLAGS} -ldflags "${GO_LDFLAGS}" ${PKG}/cmd/galaxy
  go build -o ${BIN_DIR}/galaxy ${GOBUILD_FLAGS} -ldflags "${GO_LDFLAGS}" ${PKG}/cmd/galaxy
}

function build::ipam() {
  echo "Building galaxy-ipam"
  echo "   galaxy-ipam"
  echo go build -o ${BIN_DIR}/galaxy-ipam ${GOBUILD_FLAGS} -ldflags "${GO_LDFLAGS}" ${PKG}/cmd/galaxy-ipam
  go build -o ${BIN_DIR}/galaxy-ipam ${GOBUILD_FLAGS} -ldflags "${GO_LDFLAGS}" ${PKG}/cmd/galaxy-ipam
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
    get_cni)
      build::get_basic_cni
      ;;
    galaxy)
      build::galaxy
      ;;
    galaxy-ipam)
      build::ipam
      ;;
    verify)
      build::verify
      ;;
    *)
      echo unknown arg $arg
    esac
  done
)
