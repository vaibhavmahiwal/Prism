package executor

import (
	"context"
	"io"
)

type Spec struct {
	Image   string
	Command string
	WorkDir string
	Env     []string
	Labels  map[string]string
	Mounts  []Mount
}

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

//executor runs a spec in a docker container
//exitCode is the commands owns exit code
//err can be docker img not found, docker not running, etc

type Executor interface {
	Run(ctx context.Context, spec Spec, stdout, stderr io.Writer) (exitCode int, err error)
}
