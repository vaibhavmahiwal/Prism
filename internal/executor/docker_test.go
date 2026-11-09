package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

func requireDocker(t *testing.T) *Docker {
	t.Helper()
	if os.Getenv("MINICI_DOCKER_TEST") == "" {
		t.Skip("set MINICI_DOCKER_TEST=1 to run Docker integration tests")
	}
	d, err := NewDocker()
	if err != nil {
		t.Fatalf("NewDocker() error = %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func listContainers(t *testing.T, d *Docker, label string) []types.Container {
	t.Helper()
	containers, err := d.cli.ContainerList(context.Background(), container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "mini_ci_test="+label)),
	})
	if err != nil {
		t.Fatalf("ContainerList() error = %v", err)
	}
	return containers
}

func countContainers(t *testing.T, d *Docker, label string) int {
	t.Helper()
	return len(listContainers(t, d, label))
}

func waitForRunningContainer(t *testing.T, d *Docker, label string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, c := range listContainers(t, d, label) {
			if c.State == "running" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("container with label %q never reached running state", label)
}

func TestDockerRunCancellationRemovesContainer(t *testing.T) {
	d := requireDocker(t)
	label := fmt.Sprintf("cancel-%d", time.Now().UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := d.Run(ctx, Spec{
			Image:   "alpine",
			Command: "sleep 30",
			Labels:  map[string]string{"mini_ci_test": label},
		}, io.Discard, io.Discard)
		done <- result{code: code, err: err}
	}()

	waitForRunningContainer(t, d, label)
	cancel()

	select {
	case res := <-done:
		if !errors.Is(res.err, context.Canceled) {
			t.Fatalf("Run() error = %v, want a context.Canceled error", res.err)
		}
		t.Logf("Run() returned after cancel with error: %v", res.err)
	case <-time.After(30 * time.Second):
		t.Fatal("Run() did not return within 30s of context cancel")
	}

	if got := countContainers(t, d, label); got != 0 {
		t.Fatalf("containers with label %q after cancel = %d, want 0", label, got)
	}
}
