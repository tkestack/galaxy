#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

glide up --strip-vendor
glide-vc --only-code --no-tests --no-legal-files