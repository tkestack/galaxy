#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

function create_go_path_tree() {
  local go_pkg_dir="${LOCAL_GOPATH}/src/${PKG}"
  local go_pkg_basedir=$(dirname "${go_pkg_dir}")

  mkdir -p "${go_pkg_basedir}"
  rm -f "${go_pkg_dir}"

  # TODO: This symlink should be relative.
  ln -s "${ROOT}" "${go_pkg_dir}"
}

function print_ldflag() {
  local key=${1}
  local val=${2}
  echo "-X ${version_package}.${key}=${val}"
}

function print_ldflags() {
  local -a ldflags=($(print_ldflag "BUILD_TIME" "$(date -u +'%Y-%m-%dT%H:%M:%SZ')"))
  if [[ -n ${commit-} ]]; then
    ldflags+=($(print_ldflag "GIT_COMMIT" "${commit}"))
  fi

  ldflags+=($(print_ldflag "GO_VERSION" "$(go version | awk '{print $3}')"))

  # The -ldflags parameter takes a single string, so join the output.
  echo "${ldflags[*]-}"
}