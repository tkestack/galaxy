#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

function create_go_path_tree() {
  local go_pkg_dir="${LOCAL_GOPATH}/src/${PKG_NAME}"
  local go_pkg_basedir=$(dirname "${go_pkg_dir}")

  mkdir -p "${go_pkg_basedir}"
  rm -f "${go_pkg_dir}"

  # TODO: This symlink should be relative.
  ln -s "${ROOT}" "${go_pkg_dir}"
}