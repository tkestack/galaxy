#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

readonly ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
readonly LOCAL_GOPATH="${ROOT}/go"
readonly PKG_NAME="git.code.oa.com/gaiastack/galaxy"

source ${ROOT}/hack/init.sh
create_go_path_tree

(
  export GOPATH=${LOCAL_GOPATH}
  export GOOS=linux
  cd ${LOCAL_GOPATH}/src/${PKG_NAME}/
  bad_files=$(gofmt -s -l cni/ pkg/ cmd/ tools/)
  if [[ -n "${bad_files}" ]]; then
    echo "gofmt -s -w' needs to be run on the following files: "
    echo "${bad_files}"
    exit 1
  fi
  go test -v $(glide novendor | grep -v '/go/' | grep -v '/e2e/')
  ginkgo -v e2e/k8s-vlan -- --logtostderr --v=4
)
