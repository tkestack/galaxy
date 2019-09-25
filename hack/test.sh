#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
LOCAL_GOPATH="${ROOT}/go"
PKG="git.code.oa.com/tkestack/galaxy"
echo PATH=$PATH

source ${ROOT}/hack/init.sh
create_go_path_tree

(
  export GOPATH=${LOCAL_GOPATH}
  export GOOS=linux
  cd ${LOCAL_GOPATH}/src/${PKG}/
  # Setting -p=1 because we have to execute database tests in serial. TODO use a database lock to do serial work
  # Using a len=1 golang channel doesn't fix it.
  go test -coverpkg $PKG/pkg/... -coverprofile=coverage.txt -covermode=atomic -p=1 -v $(glide novendor | grep -v '/go/' | grep -v '/e2e/')
  go tool cover -func=coverage.txt
  for i in e2e/k8s-vlan e2e/veth e2e/cni-request; do
    ginkgo -v $i -- --logtostderr --v=4
  done
)
