package executor

import (
	"fmt"

	"github.com/docker/docker/client"
)

type Docker struct {
	cli *client.Client
}

var _ Executor = (*Docker)(nil)

func NewDocker() (*Docker, error) {
	client, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create a docker client: %v", err)
	}
	return &Docker{cli: client}, nil
}
