#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

PKG="tkestack.io/galaxy"

go get github.com/onsi/ginkgo/ginkgo
export PATH=${PATH}:$(go env GOPATH)/bin
echo PATH=$PATH
export GOOS=linux
go test -race -coverpkg $PKG/pkg/... -coverprofile=coverage.txt -covermode=atomic -v ./tools/... ./cmd/... ./cni/... ./pkg/...
# go tool cover -func=coverage.txt
for i in e2e/k8s-vlan e2e/veth e2e/cni-request e2e/underlay/veth; do
  ginkgo -v $i
done
