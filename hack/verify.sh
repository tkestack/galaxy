#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
LOCAL_GOPATH="${ROOT}/go"
PKG="tkestack.io/galaxy"

source ${ROOT}/hack/init.sh
create_go_path_tree

(
  export GOPATH=${LOCAL_GOPATH}
  export GOOS=linux
  cd ${LOCAL_GOPATH}/src/${PKG}/
  bad_files=$(gofmt -s -l cni/ pkg/ cmd/ tools/)
  if [[ -n "${bad_files}" ]]; then
    echo "gofmt -s -w' needs to be run on the following files: "
    echo "${bad_files}"
    exit 1
  fi
)
