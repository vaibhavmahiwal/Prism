package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
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
	if err := d.ensureImage(ctx, spec.Image); err != nil {
		return 0, fmt.Errorf("preparing Image %q:%w", spec.Image, err)
	}
	labels := map[string]string{
		"ci_runner": "true",
	}
	for k, v := range spec.labels {
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

	if err := d.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return 0, fmt.Errorf("starting container:%w", err)
	}

	//stream logs while the container runs .Buffered channel so the
	//goroutine never block if we return early
	logsDone := make(chan error, 1)
	go func() {
		logs, err := d.cli.ContainerLogs(ctx, created.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			logsDone <- err
			return
		}
		defer logs.Close()
		_, err = stdcopy.StdCopy(stdout, stderr, logs) //splits the multiplexed streams
		logsDone <- err
	}()

	statusCh, errCh := d.cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	var exitCode int
	select {
	case err := <-errCh:
		return 0, fmt.Errorf("waiting for container to finish:%w", err)
	case status := <-statusCh:
		exitCode = int(status.StatusCode)
	}

	//Wait for the log copy to finish so no output is lost
	if err := <-logsDone; err != nil && !errors.Is(err, context.Canceled) {
		return exitCode, fmt.Errorf("reading logs: %w", err)
	}
	return exitCode, nil
}

// ensureImage pulls the image if it is not present locally
func (d *Docker) ensureImage(ctx context.Context, imageName string) error {
	_, _, err := d.cli.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil
	}
	if !client.IsErrNotFound(err) {
		return err
	}

	rc, err := d.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	//the pull only completes when the strea is fully read
	_, err = io.Copy(io.Discard, rc)
	return err
}
