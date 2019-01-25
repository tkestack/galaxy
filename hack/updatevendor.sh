#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

glide up --strip-vendor
// cherry-pick https://github.com/kubernetes/client-go/commit/da56c29d61e9015fe9240c112ca369793e4c4004 to fix bind test
git apply hack/client-go.patch