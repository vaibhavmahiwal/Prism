package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vaibhavmahiwal/Prism/internal/runner"
)

func TestRunDispatch(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		code      int
		stdoutHas string
		stderrHas string
	}{
		{name: "no args", args: nil, code: 0, stdoutHas: "Usage:"},
		{name: "help", args: []string{"help"}, code: 0, stdoutHas: "Usage:"},
		{name: "help flag", args: []string{"-h"}, code: 0, stdoutHas: "Usage:"},
		{name: "unknown command", args: []string{"bogus"}, code: 2, stderrHas: `unknown command "bogus"`},
		{name: "bad flag", args: []string{"run", "--nope"}, code: 2, stderrHas: "nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.code {
				t.Fatalf("run() = %d, want %d", got, tt.code)
			}
			if tt.stdoutHas != "" && !strings.Contains(stdout.String(), tt.stdoutHas) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.stdoutHas)
			}
			if tt.stderrHas != "" && !strings.Contains(stderr.String(), tt.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.stderrHas)
			}
		})
	}
}

func TestParseRunFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    runOptions
		wantErr string
	}{
		{
			name: "defaults",
			want: runOptions{file: ".mini-ci.yml"},
		},
		{
			name: "short aliases",
			args: []string{"-f", "ci.yml", "-w", "./app", "--keep"},
			want: runOptions{file: "ci.yml", workspace: "./app", keep: true},
		},
		{
			name: "long names",
			args: []string{"--file", "p.yml", "--workspace", "/tmp/app"},
			want: runOptions{file: "p.yml", workspace: "/tmp/app"},
		},
		{
			name: "repo and sha",
			args: []string{"--repo", "https://example.com/r.git", "--sha", "abc1234"},
			want: runOptions{file: ".mini-ci.yml", repo: "https://example.com/r.git", sha: "abc1234"},
		},
		{
			name:    "repo without sha",
			args:    []string{"--repo", "https://example.com/r.git"},
			wantErr: "--repo requires --sha",
		},
		{
			name:    "sha without repo",
			args:    []string{"--sha", "abc1234"},
			wantErr: "--sha requires --repo",
		},
		{
			name:    "repo with workspace",
			args:    []string{"--repo", "https://example.com/r.git", "--sha", "abc1234", "-w", "."},
			wantErr: "--repo and --workspace cannot be used together",
		},
		{
			name:    "positional argument",
			args:    []string{"extra"},
			wantErr: `unexpected argument "extra"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRunFlags(tt.args, io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseRunFlags() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRunFlags() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseRunFlags() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseRunFlagsHelp(t *testing.T) {
	_, err := parseRunFlags([]string{"-h"}, io.Discard)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseRunFlags(-h) error = %v, want flag.ErrHelp", err)
	}
}

func TestReportResult(t *testing.T) {
	tests := []struct {
		name      string
		res       *runner.Result
		code      int
		stderrHas string
	}{
		{
			name: "passed",
			res: &runner.Result{Status: runner.StepStatusPassed, Steps: []runner.StepResult{
				{Name: "ok", Status: runner.StepStatusPassed},
			}},
			code: 0,
		},
		{
			name: "failed",
			res: &runner.Result{Status: runner.StepStatusFailed, Steps: []runner.StepResult{
				{Name: "bad", Status: runner.StepStatusFailed, ExitCode: 7},
			}},
			code:      1,
			stderrHas: `step "bad" failed with exit code 7`,
		},
		{
			name: "errored",
			res: &runner.Result{Status: runner.StepStatusErrored, Steps: []runner.StepResult{
				{Name: "broken", Status: runner.StepStatusErrored, Err: errors.New("docker unavailable")},
			}},
			code:      2,
			stderrHas: `step "broken" error: docker unavailable`,
		},
		{
			name: "cancelled",
			res: &runner.Result{Status: runner.StepStatusCancelled, Steps: []runner.StepResult{
				{Name: "slow", Status: runner.StepStatusCancelled},
			}},
			code:      130,
			stderrHas: `step "slow" cancelled`,
		},
		{
			name:      "nil result",
			res:       nil,
			code:      2,
			stderrHas: "no result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := reportResult(tt.res, &stdout, &stderr); got != tt.code {
				t.Fatalf("reportResult() = %d, want %d", got, tt.code)
			}
			if tt.stderrHas != "" && !strings.Contains(stderr.String(), tt.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.stderrHas)
			}
		})
	}
}

func TestResolveWorkspaceLocal(t *testing.T) {
	workspace := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir, fileBase, cleanup, err := resolveWorkspace(context.Background(), runOptions{workspace: workspace}, io.Discard)
	if err != nil {
		t.Fatalf("resolveWorkspace() error = %v", err)
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if dir != abs {
		t.Errorf("dir = %q, want %q", dir, abs)
	}
	if fileBase != cwd {
		t.Errorf("fileBase = %q, want cwd %q", fileBase, cwd)
	}
	cleanup()

	defaultDir, _, _, err := resolveWorkspace(context.Background(), runOptions{}, io.Discard)
	if err != nil {
		t.Fatalf("resolveWorkspace(default) error = %v", err)
	}
	if defaultDir != cwd {
		t.Errorf("default dir = %q, want cwd %q", defaultDir, cwd)
	}
}

func TestResolveWorkspaceErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, _, _, err := resolveWorkspace(context.Background(), runOptions{workspace: missing}, io.Discard); err == nil {
		t.Fatal("resolveWorkspace(missing) error = nil")
	}

	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := resolveWorkspace(context.Background(), runOptions{workspace: file}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("resolveWorkspace(file) error = %v, want not-a-directory error", err)
	}
}
