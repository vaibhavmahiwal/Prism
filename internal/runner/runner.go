package runner

import (
	"context"
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

func Run(ctx context.Context, p *pipeline.Pipeline, ex executor.Executor, opts Options) (*Result, error) {
	res := &Result{Steps: make([]StepResult, len(p.Steps))}
	stop := false

	for i, steps := range p.Steps {
		sr := StepResult{Name: steps.Name}

		if stop || ctx.Err() != nil {
			sr.Status = StepStatusSkipped
			res.Steps[i] = sr
			continue
		}
	}

}
