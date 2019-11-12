#! /bin/bash
set -o errexit
set -o nounset
set -o pipefail

go env -w GONOSUMDB=tkestack.io/tapp
go mod tidy

#replace (
#	k8s.io/api => k8s.io/api kubernetes-1.15.4
#	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver kubernetes-1.15.4
#	k8s.io/apimachinery => k8s.io/apimachinery kubernetes-1.15.4
#	k8s.io/apiserver => k8s.io/apiserver kubernetes-1.15.4
#	k8s.io/cli-runtime => k8s.io/cli-runtime kubernetes-1.15.4
#	k8s.io/client-go => k8s.io/client-go kubernetes-1.15.4
#	k8s.io/cloud-provider => k8s.io/cloud-provider kubernetes-1.15.4
#	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap kubernetes-1.15.4
#	k8s.io/code-generator => k8s.io/code-generator kubernetes-1.15.4
#	k8s.io/component-base => k8s.io/component-base kubernetes-1.15.4
#	k8s.io/cri-api => k8s.io/cri-api kubernetes-1.15.4
#	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib kubernetes-1.15.4
#	k8s.io/kube-aggregator => k8s.io/kube-aggregator kubernetes-1.15.4
#	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager kubernetes-1.15.4
#	k8s.io/kube-proxy => k8s.io/kube-proxy kubernetes-1.15.4
#	k8s.io/kube-scheduler => k8s.io/kube-scheduler kubernetes-1.15.4
#	k8s.io/kubectl => k8s.io/kubectl kubernetes-1.15.4
#	k8s.io/kubelet => k8s.io/kubelet kubernetes-1.15.4
#	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers kubernetes-1.15.4
#	k8s.io/metrics => k8s.io/metrics kubernetes-1.15.4
#	k8s.io/sample-apiserver => k8s.io/sample-apiserver kubernetes-1.15.4
#	k8s.io/utils => k8s.io/utils master
#)
