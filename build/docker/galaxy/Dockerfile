# Tencent is pleased to support the open source community by making TKEStack
# available.
#
# Copyright (C) 2012-2019 Tencent. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use
# this file except in compliance with the License. You may obtain a copy of the
# License at
#
# https://opensource.org/licenses/Apache-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OF ANY KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations under the License.

## global scope TARGETPLATFORM
## ref: https://docs.docker.com/engine/reference/builder/#automatic-platform-args-in-the-global-scope
FROM --platform=${TARGETPLATFORM} golang:1.18.2 AS builder
ARG TARGETPLATFORM
RUN echo "building for ${TARGETPLATFORM}"
ARG WORKDIR="/go/src/tkestack.io/galaxy/"
RUN mkdir -p ${WORKDIR}
WORKDIR ${WORKDIR}
## cache dependancies if we won't change mod/sum
COPY go.mod go.sum ${WORKDIR}
RUN go mod download

COPY . ${WORKDIR}
RUN make build BINS="galaxy"
ARG TARGETARCH
ARG OUTPUTDIR="_output/bin-${TARGETARCH}"
RUN mv -f ${OUTPUTDIR}/* /
WORKDIR /



FROM --platform=${TARGETPLATFORM} centos:7

LABEL maintainer="louis(louisssgong@tencent.com)"
LABEL description="This Dockerfile is written for galaxy"
WORKDIR /root/
RUN yum install -y iproute iptables ipset
COPY --from=builder host-local loopback /opt/cni/galaxy/bin/
COPY --from=builder galaxy-k8s-sriov galaxy-k8s-vlan galaxy-underlay-veth galaxy-bridge galaxy-flannel galaxy-veth galaxy-sdn tke-route-eni /opt/cni/galaxy/bin/
COPY --from=builder galaxy /usr/bin/
