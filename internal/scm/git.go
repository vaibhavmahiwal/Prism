package scm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var commitSHA = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// this is a struct that holds the options for cloning a git repository
type CloneOptions struct {
	URL  string
	SHA  string
	Dest string
}

// func clone clones a git repo to a dest directory
func Clone(ctx context.Context, opts CloneOptions) error {
	if opts.URL == "" {
		return errors.New("clone: repository URL is required")
	}
	if opts.Dest == "" {
		return errors.New("clone: destination is required")
	}
	sha := strings.TrimSpace(opts.SHA)
	if sha == "" {
		return errors.New("clone: commit SHA is required")
	}
	if !commitSHA.MatchString(sha) {
		return fmt.Errorf("clone: invalid commit SHA %q", opts.SHA)
	}

	entries, err := os.ReadDir(opts.Dest)
	switch {
	case err == nil && len(entries) > 0:
		return fmt.Errorf("clone: destination %q is not empty", opts.Dest)
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("clone: inspecting destination: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(opts.Dest), 0o755); err != nil {
		return fmt.Errorf("clone: creating parent directory: %w", err)
	}

	if err := runGit(ctx, "clone", "--quiet", "--no-checkout", "--", opts.URL, opts.Dest); err != nil {
		return err
	}
	if err := runGit(ctx, "-C", opts.Dest, "checkout", "--quiet", "--detach", sha); err != nil {
		return err
	}
	return nil
}

func CloneToTemp(ctx context.Context, repoURL, sha string) (string, error) {
	dir, err := os.MkdirTemp("", "mini-ci-*")
	if err != nil {
		return "", fmt.Errorf("clone: creating temp directory: %w", err)
	}
	dest := filepath.Join(dir, "repo")
	if err := Clone(ctx, CloneOptions{URL: repoURL, SHA: sha, Dest: dest}); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("git %s: %w: %s", args[0], err, msg)
		}
		return fmt.Errorf("git %s: %w", args[0], err)
	}
	return nil
}
