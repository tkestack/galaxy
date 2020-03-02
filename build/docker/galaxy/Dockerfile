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

FROM BASE_IMAGE

LABEL maintainer="louis(louisssgong@tencent.com)"
LABEL description="This Dockerfile is written for galaxy"
WORKDIR /root/
RUN yum install -y iproute iptables ipset
COPY host-local loopback /opt/cni/galaxy/bin/
COPY disable-ipv6 galaxy-k8s-sriov galaxy-k8s-vlan galaxy-bridge galaxy-flannel galaxy-veth galaxy-sdn tke-route-eni /opt/cni/galaxy/bin/
COPY galaxy /usr/bin/
