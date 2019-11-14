#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

PKG="tkestack.io/galaxy"
echo PATH=$PATH

export GOOS=linux
# Setting -p=1 because we have to execute database tests in serial. TODO use a database lock to do serial work
# Using a len=1 golang channel doesn't fix it.
go test -coverpkg $PKG/pkg/... -coverprofile=coverage.txt -covermode=atomic -p=1 -v ./tools/... ./cmd/... ./cni/... ./pkg/...
go tool cover -func=coverage.txt
for i in e2e/k8s-vlan e2e/veth e2e/cni-request; do
ginkgo -v $i
done
