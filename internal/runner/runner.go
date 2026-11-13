package runner

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/vaibhavmahiwal/Prism/internal/executor"
	"github.com/vaibhavmahiwal/Prism/internal/pipeline"
)

type StepStatus string

const (
	StepStatusPassed    StepStatus = "passed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
	StepStatusCancelled StepStatus = "cancelled"
	StepStatusErrored   StepStatus = "errored"
)

type StepResult struct {
	Name     string
	Status   StepStatus
	ExitCode int
	Duration time.Duration
	Err      error
}

type Options struct {
	Workspace string
	Stdout    io.Writer
	Stderr    io.Writer
}

type Result struct {
	Steps []StepResult
}

func Run(ctx context.Context, p *pipeline.Pipeline, ex executor.Executor, opts Options) (*Result, error) {
	res := &Result{Steps: make([]StepResult, len(p.Steps))}

	for i, steps := range p.Steps {
		sr := StepResult{Name: steps.Name}

		if ctx.Err() != nil {
			sr.Status = StepStatusSkipped
			res.Steps[i] = sr
			continue
		}

		fmt.Fprintf(opts.Stdout, "==>[%d/%d]/%s\n", i+1, len(p.Steps), steps.Name)

		start := time.Now()
		code, err := ex.Run(ctx, executor.Spec{
			Image:   p.Image,
			Command: steps.Run,
			WorkDir: "/workspace",
			Mounts:  []executor.Mount{{Source: opts.Workspace, Target: "/workspace"}},
		}, opts.Stdout, opts.Stderr)
		sr.Duration = time.Since(start)
		sr.ExitCode = code

		if err != nil {
			sr.Status = StepStatusErrored
			sr.Err = err
		} else if code != 0 {
			sr.Status = StepStatusFailed
		} else {
			sr.Status = StepStatusPassed
		}
		res.Steps[i] = sr

		if code != 0 || err != nil {
			break
		}
	}
	return res, nil
}
