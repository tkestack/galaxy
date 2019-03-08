ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd -P)
protoc -I ${ROOT}/pkg/ipam/cloudprovider/rpc ${ROOT}/pkg/ipam/cloudprovider/rpc/*.proto --go_out=plugins=grpc:pkg/ipam/cloudprovider/rpc
