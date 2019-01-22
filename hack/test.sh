#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
LOCAL_GOPATH="${ROOT}/go"
PKG="git.code.oa.com/gaiastack/galaxy"

source ${ROOT}/hack/init.sh
create_go_path_tree

(
  export GOPATH=${LOCAL_GOPATH}
  export GOOS=linux
  cd ${LOCAL_GOPATH}/src/${PKG}/
  go test -coverpkg $PKG/pkg/... -coverprofile=coverage.txt -covermode=atomic -v $(glide novendor | grep -v '/go/' | grep -v '/e2e/')
  go tool cover -func=coverage.txt
  for i in e2e/k8s-vlan e2e/veth; do
    ginkgo -v $i -- --logtostderr --v=4
  done
)
