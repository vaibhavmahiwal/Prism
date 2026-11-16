package runner

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/vaibhavmahiwal/Prism/internal/executor"
	"github.com/vaibhavmahiwal/Prism/internal/pipeline"
)

type fakeExecutor struct {
	codes []int
	errs  []error
	specs []executor.Spec
}

func (f *fakeExecutor) Run(_ context.Context, spec executor.Spec, _, _ io.Writer) (int, error) {
	i := len(f.specs)
	f.specs = append(f.specs, spec)
	return f.codes[i], f.errs[i]
}

func TestRunRecordsResultsAndStopsAfterFailure(t *testing.T) {
	ex := &fakeExecutor{codes: []int{0, 1, 0}, errs: []error{nil, nil, nil}}
	p := &pipeline.Pipeline{
		Image: "alpine",
		Steps: []pipeline.Step{
			{Name: "first", Run: "echo first"},
			{Name: "second", Run: "false"},
			{Name: "third", Run: "echo third"},
		},
	}

	result, err := Run(context.Background(), p, ex, Options{Workspace: "C:\\workspace"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Status != StepStatusFailed {
		t.Fatalf("overall status = %q, want %q", result.Status, StepStatusFailed)
	}
	statuses := []StepStatus{result.Steps[0].Status, result.Steps[1].Status, result.Steps[2].Status}
	want := []StepStatus{StepStatusPassed, StepStatusFailed, StepStatusSkipped}
	for i := range want {
		if statuses[i] != want[i] {
			t.Errorf("step %d status = %q, want %q", i, statuses[i], want[i])
		}
	}
	if len(ex.specs) != 2 {
		t.Fatalf("executor calls = %d, want 2", len(ex.specs))
	}
	if ex.specs[0].Image != "alpine" || ex.specs[0].Command != "echo first" {
		t.Errorf("first spec = %#v", ex.specs[0])
	}
	if len(ex.specs[0].Mounts) != 1 || ex.specs[0].Mounts[0].Source != "C:\\workspace" {
		t.Errorf("first mounts = %#v", ex.specs[0].Mounts)
	}
}

func TestRunStoresInfrastructureErrorsInResult(t *testing.T) {
	ex := &fakeExecutor{
		codes: []int{0, 0},
		errs:  []error{errors.New("docker unavailable"), nil},
	}
	p := &pipeline.Pipeline{
		Image: "alpine",
		Steps: []pipeline.Step{
			{Name: "first", Run: "echo first"},
			{Name: "second", Run: "echo second"},
		},
	}

	result, err := Run(context.Background(), p, ex, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StepStatusErrored || result.Steps[0].Status != StepStatusErrored {
		t.Fatalf("result statuses = %q, %q", result.Status, result.Steps[0].Status)
	}
	if result.Steps[0].Err == nil || result.Steps[1].Status != StepStatusSkipped {
		t.Fatalf("result = %#v", result)
	}
}
