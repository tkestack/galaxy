#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

flags=${debug:+"-v"}
ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
LOCAL_GOPATH="${ROOT}/go"
PKG=git.code.oa.com/tkestack/galaxy

source ${ROOT}/hack/init.sh
create_go_path_tree

export GOPATH=${ROOT}/go
export GOOS=linux
function cleanup() {
	rm ${ROOT}/go/src/${PKG}
}
trap cleanup EXIT

commit=${commit-$(git log --first-parent -1 --oneline | awk '{print $1}')}
version_package=${PKG}/pkg/utils/ldflags
# build galaxy-ipam
echo "Building galaxy-ipam"
echo "   galaxy-ipam"
echo go build -o bin/galaxy-ipam $flags -ldflags "$(print_ldflags)" ${PKG}/cmd/galaxy-ipam
go build -o bin/galaxy-ipam $flags -ldflags "$(print_ldflags)" ${PKG}/cmd/galaxy-ipam
