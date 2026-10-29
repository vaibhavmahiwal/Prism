package executor

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
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

func (d *Docker) Close() error {
	return d.cli.Close()
}

func (d *Docker) Run(ctx context.Context, spec Spec, stdout, stderr io.Writer) (int, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	//this handles the case when the image is not present in the local docker reg
	if err := d.InsureImage(ctx, spec.Image); err != nil {
		return 0, fmt.Errorf("failed to ensure image %s:%v", spec.Image, err)
	}
	labels := map[string]string{
		"ci_runner": "true",
	}
	for k, v := range spec.Labels {
		labels[k] = v
	}
	created, err := d.cli.ContainerCreate(ctx, &container.Config{
		Image:      spec.Image,
		Cmd:        []string{"sh", "-c", spec.Command},
		WorkingDir: spec.WorkDir,
		Env:        spec.Env,
		Labels:     labels,
		Tty:        false,
	},
		&container.HostConfig{},
		nil, nil, "")
	if err != nil {
		return 0, fmt.Errorf("creating container: %w", err)
	}
	//Always cleanup,using a fresh context :if ctx was cancelled
	//calls made with it would fail and container would leak
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = d.cli.ContainerRemove(cleanupCtx, created.ID,
			container.RemoveOptions{Force: true, RemoveVolumes: true})
	}()

}
