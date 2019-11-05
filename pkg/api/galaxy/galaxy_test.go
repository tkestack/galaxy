/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package galaxy

import (
	"fmt"
	"testing"

	"tkestack.io/galaxy/pkg/api/k8s"
)

func TestCniRequestToPodRequest(t *testing.T) {
	// config, err := json.Marshal(CNIRequest{Config: []byte("{\"capabilities\":{\"portMappings\":true},\"cniVersion\":\"\",\"name\":\"\",\"runtimeConfig\":{\"portMappings\":[{\"hostPort\":30001,\"containerPort\":80,\"protocol\":\"tcp\",\"hostIP\":\"\"}]}}")})
	pr, err := CniRequestToPodRequest([]byte(`{
    "env": {
        "CNI_COMMAND": "ADD",
        "CNI_CONTAINERID": "ctn1",
        "CNI_NETNS": "/var/run/netns/ctn",
        "CNI_IFNAME": "eth0",
        "CNI_PATH": "/opt/cni/bin",
        "CNI_ARGS": "K8S_POD_NAMESPACE=demo;K8S_POD_NAME=app;K8S_POD_INFRA_CONTAINER_ID=ctn1"
    },
    "config":"eyJjYXBhYmlsaXRpZXMiOnsicG9ydE1hcHBpbmdzIjp0cnVlfSwiY25pVmVyc2lvbiI6IiIsIm5hbWUiOiIiLCJydW50aW1lQ29uZmlnIjp7InBvcnRNYXBwaW5ncyI6W3siaG9zdFBvcnQiOjMwMDAxLCJjb250YWluZXJQb3J0Ijo4MCwicHJvdG9jb2wiOiJ0Y3AiLCJob3N0SVAiOiIifV19fQ=="
}`))
	if err != nil {
		t.Error(err)
	}
	if len(pr.Ports) != 1 {
		t.Fatal(pr.Ports)
	}
	if fmt.Sprintf("%+v", pr.Ports[0]) != "{HostPort:30001 ContainerPort:80 Protocol:tcp HostIP: PodName: PodIP:}" {
		t.Fatalf("%+v", pr.Ports[0])
	}
}
func TestCleanDuplicate(t *testing.T) {
	ports := cleanDuplicate([]k8s.Port{
		{ContainerPort: 80, Protocol: "tcp"},
		{ContainerPort: 80, Protocol: "udp"},
		{ContainerPort: 80, Protocol: "tcp"},
		{ContainerPort: 81, Protocol: "tcp"},
	})
	if fmt.Sprintf("%+v", ports) != "[{HostPort:0 ContainerPort:80 Protocol:tcp HostIP: PodName: PodIP:} {HostPort:0 ContainerPort:80 Protocol:udp HostIP: PodName: PodIP:} {HostPort:0 ContainerPort:81 Protocol:tcp HostIP: PodName: PodIP:}]" {
		t.Fatalf("%+v", ports)
	}
}
