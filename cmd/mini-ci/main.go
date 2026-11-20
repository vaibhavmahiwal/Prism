package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/vaibhavmahiwal/Prism/internal/executor"
	"github.com/vaibhavmahiwal/Prism/internal/pipeline"
	"github.com/vaibhavmahiwal/Prism/internal/runner"
	"github.com/vaibhavmahiwal/Prism/internal/scm"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func usage(w io.Writer) {
	_, _ = io.WriteString(w, `Usage: mini-ci run [flags]

Flags:
  -f, --file string        Path to pipeline file (default ".mini-ci.yml")
  -w, --workspace string   Local workspace directory (default current directory)
      --repo string        Git repository URL to clone (requires --sha)
      --sha string         Git commit SHA to check out
      --keep               Keep cloned temporary workspace after run

Examples:
  mini-ci run
  mini-ci run -f ci/pipeline.yml -w ./my-app
  mini-ci run --repo https://github.com/org/repo.git --sha a1b2c3d --keep
`)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stdout)
		return 0
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	case "run":
		return runCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
}

type runOptions struct {
	file      string
	workspace string
	repo      string
	sha       string
	keep      bool
}

// function to parse the flags for the run command,
//
//	it takes the args and a writer to write errors to
func parseRunFlags(args []string, stderr io.Writer) (runOptions, error) {
	var opts runOptions
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}

	fs.StringVar(&opts.file, "file", ".mini-ci.yml", "Path to pipeline file")
	fs.StringVar(&opts.file, "f", ".mini-ci.yml", "")
	fs.StringVar(&opts.workspace, "workspace", "", "")
	fs.StringVar(&opts.workspace, "w", "", "")
	fs.StringVar(&opts.repo, "repo", "", "")
	fs.StringVar(&opts.sha, "sha", "", "")
	fs.BoolVar(&opts.keep, "keep", false, "")

	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}
	if fs.NArg() > 0 {
		return runOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	switch {
	case opts.repo != "" && opts.sha == "":
		return runOptions{}, errors.New("--repo requires --sha")
	case opts.sha != "" && opts.repo == "":
		return runOptions{}, errors.New("--sha requires --repo")
	case opts.repo != "" && opts.workspace != "":
		return runOptions{}, errors.New("--repo and --workspace cannot be used together")
	}
	return opts, nil
}

func runCommand(args []string, stdout, stderr io.Writer) int {
	opts, err := parseRunFlags(args, stderr)

	if errors.Is(err, flag.ErrHelp) {
		usage(stdout)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dir, fileBase, cleanup, err := resolveWorkspace(ctx, opts, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		if ctx.Err() != nil {
			return 130
		}
		return 2
	}
	defer cleanup()
	filePath := opts.file
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(fileBase, filePath)
	}

	p, err := pipeline.ParseFile(filePath)
	if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		if ctx.Err() != nil {
			return 130
		}
		return 2
	}

	ex, err := executor.NewDocker()
	if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		return 2
	}
	defer func() { _ = ex.Close() }()

	res, err := runner.Run(ctx, p, ex, runner.Options{
		Workspace: dir,
		Stdout:    stdout,
		Stderr:    stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "mini-ci: %v\n", err)
		if ctx.Err() != nil {
			return 130
		}
		return 2
	}

	return reportResult(res, stdout, stderr)
}

func resolveWorkspace(ctx context.Context, opts runOptions, stderr io.Writer) (dir, fileBase string, cleanup func(), err error) {
	if opts.repo != "" {
		root, cloneErr := scm.CloneToTemp(ctx, opts.repo, opts.sha)
		if cloneErr != nil {
			return "", "", nil, cloneErr
		}
		dir = filepath.Join(root, "repo")
		fileBase = dir
		cleanup = func() {
			if opts.keep {
				fmt.Fprintf(stderr, "keeping cloned workspace at %q\n", root)
				return
			}
			_ = os.RemoveAll(root)
		}
		return dir, fileBase, cleanup, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", "", nil, fmt.Errorf("resolving current directory: %w", err)
	}
	base := opts.workspace
	if base == "" {
		base = cwd
	}
	dir, err = filepath.Abs(base)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolving absolute path of workspace %q: %w", base, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", "", nil, fmt.Errorf("workspace %q: %w", dir, err)
	}
	if !info.IsDir() {
		return "", "", nil, fmt.Errorf("workspace %q is not a directory", dir)
	}
	return dir, cwd, func() {}, nil
}

// this is used to report the result of the run command ,
// it takes the result , stdout and a writer to write errors to
// and return the exit code of the command
func reportResult(res *runner.Result, stdout, stderr io.Writer) int {
	if res == nil {
		fmt.Fprintln(stderr, "mini-ci: no result")
		return 2
	}
	if err := res.WriteSummary(stdout); err != nil {
		fmt.Fprintf(stderr, "mini-ci: writing summary: %v\n", err)
		return 2
	}
	for _, step := range res.Steps {
		switch step.Status {
		case runner.StepStatusFailed:
			fmt.Fprintf(stderr, "step %q failed with exit code %d\n", step.Name, step.ExitCode)
		case runner.StepStatusErrored:
			fmt.Fprintf(stderr, "step %q error: %v\n", step.Name, step.Err)
		case runner.StepStatusCancelled:
			fmt.Fprintf(stderr, "step %q cancelled\n", step.Name)
		}
	}
	switch res.Status {
	case runner.StepStatusPassed:
		return 0
	case runner.StepStatusFailed:
		return 1
	case runner.StepStatusCancelled:
		return 130
	default:
		return 2
	}
}
