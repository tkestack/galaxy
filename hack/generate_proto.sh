ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
protoc -I ${ROOT}/pkg/ipam/cloudprovider ${ROOT}/pkg/ipam/cloudprovider/*.proto --go_out=plugins=grpc:pkg/ipam/cloudprovider
