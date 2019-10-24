package docker

import (
	"fmt"
	"time"

	dockerapi "github.com/docker/engine-api/client"
	dockertypes "github.com/docker/engine-api/types"
	"golang.org/x/net/context"
	glog "k8s.io/klog"
)

var (
	// defaultTimeout is the default timeout of short running docker operations.
	defaultTimeout = 2 * time.Minute
	unixSocket     = "unix:///var/run/docker.sock"
)

type DockerInterface struct {
	timeout time.Duration
	client  *dockerapi.Client
}

// NewDockerInterface creates an DockerInterface
func NewDockerInterface() (*DockerInterface, error) {
	dockerCli, err := getDockerClient(unixSocket)
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

func (d *DockerInterface) InspectContainer(id string) (*dockertypes.ContainerJSON, error) {
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
