# mini-ci

A small self-hosted CI runner written in Go. A GitHub webhook hits the server, the server queues a job, a worker runs the steps from a YAML pipeline file inside Docker containers, and logs and status are available through an API and a simple web page.

See [PLAN.md](PLAN.md) for the full project plan.

## Status

Phase 1 in progress. Check the boxes below as each item is completed.

## How it works

```
GitHub --webhook--> HTTP server --> job queue --> worker pool
                        |                             |
                        v                             v
                   SQLite/Postgres               Docker Engine
                   (jobs, steps, logs)        (one container per step)
                        ^                             |
                        '------ live logs <-----------'
```

Example pipeline (`.mini-ci.yml`):

```yaml
name: build-and-test
image: golang:1.26
steps:
  - name: test
    run: go test ./...
  - name: build
    run: go build ./...
```

## Requirements

| Tool | Version | Check |
|---|---|---|
| Go | 1.26+ | `go version` |
| Docker | 28+ | `docker version` |
| git | 2.50+ | `git --version` |

On Windows, use WSL2 for parity with the Linux deployment target. The Docker SDK needs `npipe:////./pipe/docker_engine`, bind mounts need Windows-style paths, and `SIGTERM` does not exist on native Windows.

## Phase checklist

### Phase 1: Pipeline parser

Goal: read a pipeline YAML into structs and validate it. No Docker, no server.

- [ ] Create `go.mod` (`go mod init github.com/vaibh/mini-ci`).
- [ ] Add `gopkg.in/yaml.v3` (`go get gopkg.in/yaml.v3`).
- [ ] Define `Pipeline` and `Step` structs with YAML tags in `internal/pipeline/types.go`.
- [ ] Implement `Parse([]byte) (*Pipeline, error)` in `internal/pipeline/parser.go`.
- [ ] Validate that `image` is present and non-empty.
- [ ] Validate that at least one step exists.
- [ ] Validate that step names are non-empty and unique.
- [ ] Return errors that name the offending field (e.g. `step 2: name is required`).
- [ ] Add table-driven tests covering: valid file, missing image, empty steps list, duplicate step names.
- [ ] Run `go test ./internal/pipeline`.

Done when: `go test ./...` passes and every invalid case produces a clear, field-specific error.

### Phase 2: Run one command in one container

Goal: the smallest useful Docker integration.

- [ ] Define the `Executor` interface in `internal/executor/executor.go`, e.g. `Run(ctx, spec) (exitCode int, err error)`.
- [ ] Implement `internal/executor/docker.go` using the Docker Engine Go SDK.
- [ ] Pull the image, create the container, start it, wait, read the exit code, remove the container.
- [ ] Print container output to the terminal.
- [ ] Handle the multiplexed stdout/stderr stream format.
- [ ] Add a throwaway `cmd/spike/main.go` that runs `echo hello` in `alpine`.

Done when: the spike prints `hello` and `docker ps -a` shows no leftover container.

### Phase 3: Run a whole pipeline from the command line

Goal: `mini-ci run .mini-ci.yml` works with no server.

- [ ] Implement `internal/scm/git.go` to clone a repo at a given commit.
- [ ] Support a local directory as the workspace.
- [ ] Mount the workspace into the container.
- [ ] Run steps in order and stop at the first non-zero exit code.
- [ ] Report which step failed.
- [ ] Clean up the container and temp workspace on success, failure, and Ctrl+C.
- [ ] Add the `run` subcommand in `cmd/mini-ci/main.go`.

Done when: a passing pipeline exits 0, a failing one exits 1 and names the failing step, and Ctrl+C leaves nothing running.

### Phase 4: Store and job model

Goal: durable state behind an interface.

- [ ] Write `migrations/001_init.sql` with the `jobs`, `steps`, and `log_lines` tables.
- [ ] Define model structs and the `Store` interface in `internal/store/store.go`: create job, get job, list jobs, update status, append log lines, list log lines.
- [ ] Implement it with SQLite in `internal/store/sqlite.go`.
- [ ] Add `internal/store/store_test.go` against a temporary database.
- [ ] Test status transitions (`queued` -> `running` -> `passed`/`failed`/`cancelled`).

Done when: store tests pass, including status transitions, and no test depends on a shared database file.

### Phase 5: HTTP server and webhook

Goal: a push event creates a queued job.

- [ ] `POST /webhook` verifies `X-Hub-Signature-256` HMAC using `hmac.Equal` (constant-time compare).
- [ ] Reject bad signatures with 401.
- [ ] Parse the push payload: repo, clone URL, SHA, branch.
- [ ] Insert a `queued` job and return 202 immediately.
- [ ] Add `GET /healthz` returning OK.
- [ ] Load the webhook secret from config, not source.
- [ ] Test with a signed `curl` payload, an unsigned payload, and a tampered payload.

Done when: a signed test payload creates a job row, unsigned or tampered payloads are rejected, and the handler returns without running anything.

### Phase 6: Queue, workers, cancellation, shutdown

Goal: jobs run in the background, safely.

- [ ] Implement a queue in `internal/queue/queue.go` (buffered channel, or poll the DB for `queued` jobs).
- [ ] Implement a pool of N worker goroutines in `internal/worker/pool.go`.
- [ ] Each worker: mark `running`, run the pipeline through `Executor`, record step results, mark `passed` or `failed`.
- [ ] Thread `context.Context` through every layer.
- [ ] Add a per-job timeout.
- [ ] Add a cancel path that actually stops the container (cancelling a Go context does not stop a container).
- [ ] Graceful shutdown: on signal, stop taking jobs, then wait for or cancel in-flight ones.
- [ ] Implement `internal/executor/fake.go` with no Docker dependency.
- [ ] Test that N jobs run concurrently and never more than N.
- [ ] Test that a cancelled job ends as `cancelled`.
- [ ] Test that shutdown leaves no job stuck in `running`.

Done when: the fake-executor pool test proves the concurrency bound, cancellation works end to end, and shutdown leaves no stuck job.

### Phase 7: Logs and the API

Goal: see what happened, live.

- [ ] Copy container output line by line into `log_lines` as it arrives.
- [ ] `GET /jobs` lists jobs.
- [ ] `GET /jobs/{id}` returns one job with its steps.
- [ ] `POST /jobs/{id}/cancel` cancels a running job.
- [ ] `GET /jobs/{id}/logs` streams new lines with server-sent events and replays stored lines first.
- [ ] Build `web/index.html` to list jobs and show logs.

Done when: you send a test webhook, open the page, and watch logs appear while the job runs.

### Phase 8: Reliability

Goal: it behaves under failure.

- [ ] Crash recovery: on startup, re-queue or fail any job left in `running` with a clear reason.
- [ ] Label every container the runner creates.
- [ ] Remove stale labeled containers at startup.
- [ ] Set CPU and memory limits.
- [ ] Enforce a maximum log size per step.
- [ ] Decide retry behavior for clone failures and Docker being unreachable, and record the reason on the job.
- [ ] Test with `kill -9` mid-job, then restart.

Done when: killing the server mid-job and restarting leaves no stuck job and no orphan container.

### Phase 9: Packaging, tests, docs

Goal: someone else can run it.

- [ ] Multi-stage `Dockerfile`.
- [ ] `docker-compose.yml` starting the server and mounting the Docker socket.
- [ ] `Makefile` targets: `build`, `test`, `lint`, `run`.
- [ ] Add `.mini-ci.yml` so mini-ci builds and tests itself.
- [ ] Write install and webhook configuration instructions.
- [ ] Document security caveats.
- [ ] Fill test coverage gaps: parser, store, worker pool with fake executor, webhook signatures.
- [ ] Run `docker compose up` on a clean machine.

Done when: `docker compose up` works from scratch, `go test ./...` passes, and mini-ci's own pipeline passes on its own repo.

### Phase 10: Stretch goals (pick any)

- [ ] Kubernetes Job `Executor` implementation.
- [ ] Prometheus metrics: queue depth, job duration, failure rate.
- [ ] Per-job environment variables and a simple secrets mechanism.
- [ ] Cache directories between runs.
- [ ] Post commit status checks back to GitHub.

## Data model

| Table | Columns |
|---|---|
| `jobs` | id, repo, clone_url, commit_sha, branch, status, created_at, started_at, finished_at |
| `steps` | id, job_id, name, position, status, exit_code, started_at, finished_at |
| `log_lines` | id, step_id, seq, text, created_at |

Job status values: `queued`, `running`, `passed`, `failed`, `cancelled`.

## Development commands

```sh
go test ./...          # run all tests
go build ./...         # build all packages
go run ./cmd/mini-ci   # run the server
go vet ./...           # static checks
```

## Security

This runs arbitrary code from repositories. Run it locally or on a throwaway machine, never expose it publicly. Mounting the Docker socket gives the runner root-equivalent access to the host. There is no authentication on the API or web page beyond the webhook signature, and no secrets management.

## Repository layout

```
cmd/mini-ci/        wiring: config, store, queue, workers, server, shutdown
internal/pipeline/  YAML parsing and validation          (Phase 1)
internal/executor/  Executor interface + Docker/fake     (Phases 2-3, 6)
internal/scm/       git clone and webhook signatures     (Phase 3, 5)
internal/store/     persistence behind an interface      (Phase 4)
internal/queue/     job queue                            (Phase 6)
internal/worker/    worker pool, cancellation, shutdown  (Phase 6)
internal/api/       HTTP layer and SSE                   (Phases 5, 7)
internal/config/    configuration
migrations/         SQL schema
web/                plain HTML + JS
docs/NOTES.md       running log of bugs and decisions
```
