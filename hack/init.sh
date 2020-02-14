#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

PKG=tkestack.io/galaxy

function init::print_ldflag() {
  version_package=${PKG}/pkg/utils/ldflags
  local key=${1}
  local val=${2}
  echo "-X ${version_package}.${key}=${val}"
}

function init::print_ldflags() {
  local -a ldflags=($(init::print_ldflag "BUILD_TIME" "$(date -u +'%Y-%m-%dT%H:%M:%SZ')"))
  if [[ -n ${GIT_COMMIT-} ]]; then
    ldflags+=($(init::print_ldflag "GIT_COMMIT" "${GIT_COMMIT}"))
  fi

  ldflags+=($(init::print_ldflag "GO_VERSION" "$(go version | awk '{print $3}')"))

  # The -ldflags parameter takes a single string, so join the output.
  echo "${ldflags[*]-}"
}

function init::docker_builded() {
    if [[ "$(go env GOOS)" == "darwin" ]] && [[ "$1" = "binary_"* ]]; then
      echo "Building $1 via docker"
      docker run --rm -v $(pwd):/go/src/$PKG -v $GOPATH/pkg:/go/pkg -w /go/src/$PKG golang:1.13 hack/build.sh "$1"
      return
    fi
    false
}

VERSION=$(cat VERSION)
REGISTRY=docker.io/tkestack
GIT_COMMIT=$(git log --first-parent -1 --oneline | awk '{print $1}')
