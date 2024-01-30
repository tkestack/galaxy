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
package docker

import (
	"fmt"
	dockerapi "github.com/docker/engine-api/client"
	dockertypes "github.com/docker/engine-api/types"
	"golang.org/x/net/context"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	glog "k8s.io/klog"
	"os"
	"time"
)

var (
	// defaultTimeout is the default timeout of short running docker operations.
	defaultTimeout = 2 * time.Minute
)

type DockerInterface struct {
	timeout          time.Duration
	client           *dockerapi.Client
	containerdClient criapi.RuntimeServiceClient
}

// NewDockerInterface creates an DockerInterface
func NewDockerInterface() (*DockerInterface, error) {
	if os.Getenv("CONTAINERD_HOST") != "" {
		containerdClient, err := newContainerdClient()
		if err != nil {
			return nil, err
		}
		return &DockerInterface{
			timeout:          defaultTimeout,
			containerdClient: containerdClient,
		}, nil
	}
	dockerCli, err := getDockerClient("")
	if err != nil {
		return nil, err
	}
	return &DockerInterface{
		client:  dockerCli,
		timeout: defaultTimeout,
	}, nil
}

// Get a *dockerapi.Client, either using the endpoint passed in, or using
// DOCKER_HOST, DOCKER_TLS_VERIFY, and DOCKER_CERT path per their spec
func getDockerClient(dockerEndpoint string) (*dockerapi.Client, error) {
	if len(dockerEndpoint) > 0 {
		glog.Infof("Connecting to docker on %s", dockerEndpoint)
		return dockerapi.NewClient(dockerEndpoint, "", nil, nil)
	}
	return dockerapi.NewEnvClient()
}

func getTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

func (d *DockerInterface) DockerInspectContainer(id string) (*dockertypes.ContainerJSON, error) {
	ctx, cancel := getTimeoutContext()
	defer cancel()
	containerJSON, err := d.client.ContainerInspect(ctx, id)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		if dockerapi.IsErrContainerNotFound(err) {
			return nil, ContainerNotFoundError{id}
		}
		return nil, err
	}
	return &containerJSON, nil
}

func (d *DockerInterface) ContainedInspectContainer(id string) (*criapi.PodSandboxStatus, error) {
	ctx, cancel := getTimeoutContext()
	defer cancel()
	if os.Getenv("CONTAINERD_HOST") != "" {
		request := &criapi.PodSandboxStatusRequest{
			PodSandboxId: id,
			Verbose:      true,
		}
		resp, err := d.containerdClient.PodSandboxStatus(ctx, request)
		if err != nil {
			return nil, err
		}
		return resp.Status, nil
	}
	return nil, fmt.Errorf("CONTAINERD_HOST is not configured")
}

// contextError checks the context, and returns error if the context is timeout.
func contextError(ctx context.Context) error {
	if ctx.Err() == context.DeadlineExceeded {
		return operationTimeout{err: ctx.Err()}
	}
	return ctx.Err()
}

// operationTimeout is the error returned when the docker operations are timeout.
type operationTimeout struct {
	err error
}

func (e operationTimeout) Error() string {
	return fmt.Sprintf("operation timeout: %v", e.err)
}

// containerNotFoundError is the error returned by InspectContainer when container not found. We
// add this error type for testability. We don't use the original error returned by engine-api
// because dockertypes.containerNotFoundError is private, we can't create and inject it in our test.
type ContainerNotFoundError struct {
	ID string
}

func (e ContainerNotFoundError) Error() string {
	return fmt.Sprintf("no such container: %q", e.ID)
}
