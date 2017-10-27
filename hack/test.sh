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
  cd ${LOCAL_GOPATH}/src/${PKG_NAME}/
  go test -v $(glide novendor | grep -v '/go/')
)
