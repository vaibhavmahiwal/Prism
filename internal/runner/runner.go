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
	Status StepStatus
	Steps  []StepResult
}

func Run(ctx context.Context, p *pipeline.Pipeline, ex executor.Executor, opts Options) (*Result, error) {
	if p == nil {
		return nil, fmt.Errorf("pipeline is nil")
	}
	if ex == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}

	res := &Result{Status: StepStatusPassed, Steps: make([]StepResult, len(p.Steps))}
	for i, steps := range p.Steps {
		sr := StepResult{Name: steps.Name}

		if ctx.Err() != nil {
			sr.Status = StepStatusCancelled
			res.Steps[i] = sr
			markSkipped(res.Steps, i+1)
			res.Status = StepStatusCancelled
			break
		}

		fmt.Fprintf(stdout, "==>[%d/%d]/%s\n", i+1, len(p.Steps), steps.Name)

		start := time.Now()
		code, err := ex.Run(ctx, executor.Spec{
			Image:   p.Image,
			Command: steps.Run,
			WorkDir: "/workspace",
			Mounts:  []executor.Mount{{Source: opts.Workspace, Target: "/workspace"}},
		}, stdout, opts.Stderr)
		sr.Duration = time.Since(start)
		sr.ExitCode = code

		if ctx.Err() != nil {
			sr.Status = StepStatusCancelled
			sr.Err = err
			res.Status = StepStatusCancelled
		} else if err != nil {
			sr.Status = StepStatusErrored
			sr.Err = err
			res.Status = StepStatusErrored
		} else if code != 0 {
			sr.Status = StepStatusFailed
			res.Status = StepStatusFailed
		} else {
			sr.Status = StepStatusPassed
		}
		res.Steps[i] = sr

		if sr.Status != StepStatusPassed {
			markSkipped(res.Steps, i+1)
			break
		}
	}
	return res, nil
}

func markSkipped(steps []StepResult, start int) {
	for i := start; i < len(steps); i++ {
		steps[i].Status = StepStatusSkipped
	}
}

func (r *Result) WriteSummary(w io.Writer) error {
	if r == nil {
		return fmt.Errorf("result is nil")
	}
	if w == nil {
		return fmt.Errorf("summary writer is nil")
	}

	if _, err := fmt.Fprintln(w, "\nNAME\tSTATUS\tDURATION"); err != nil {
		return err
	}
	for _, step := range r.Steps {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", step.Name, step.Status, step.Duration); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "overall\t%s\n", r.Status)
	return err
}
