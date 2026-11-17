# Architecture

Status: Living · Scope: component layout, runtime flows, concurrency, failure paths.

The authoritative task list lives in [../README.md](../README.md); phase status lives in [ROADMAP.md](ROADMAP.md); rationale lives in [DECISIONS.md](DECISIONS.md).

## Current state

Implemented through Phase 2, with Phase 3 half done:

| Area | State |
|---|---|
| Pipeline parser and validation | Done |
| Docker executor (`Executor` + Docker implementation) | Done |
| Git clone at a commit | Done |
| Pipeline run loop (`runner.Run`) | Done, unit-tested with a fake executor |
| CLI (`cmd/mini-ci`) | Stub |
| Store, queue, worker pool, HTTP API, web UI | Not started |

## Component diagram

```
          today (solid)                 planned (dashed)

  cli  -->  pipeline.ParseFile
    |            |
    |            v
    |       runner.Run  -->  executor.Docker  -->  Docker Engine
    |            ^                |
    |            |                '-- pulls image, creates/starts/waits/removes
    |       scm.CloneToTemp
    |
    '-- (planned)
         GitHub --webhook--> api --> store --> queue --> worker pool --> runner.Run
                                  ^                        |
                                  '------ logs / status <---'
                                  |
                                  '--> SSE --> web page
```

## Components

| Package | Responsibility | Depends on | Status |
|---|---|---|---|
| `internal/pipeline` | Parse and validate `.mini-ci.yml` into `Pipeline`/`Step` | `gopkg.in/yaml.v3` | Done |
| `internal/executor` | `Executor` interface, `Spec`, Docker implementation | Docker Engine SDK | Done |
| `internal/scm` | Clone a repo, check out an exact commit | `git` via `os/exec` | Done |
| `internal/runner` | Run pipeline steps in order through `Executor` | `pipeline`, `executor` | Done |
| `cmd/mini-ci` | CLI wiring: flags, workspace, signals, exit codes | all of the above | Stub |
| `internal/store` | Jobs, steps, log lines behind a `Store` interface | SQLite driver | Planned |
| `internal/queue` | Feed queued jobs to workers | `store` or channel | Planned |
| `internal/worker` | Pool, per-job context, status transitions, shutdown | `Executor`, `Store`, `runner` | Planned |
| `internal/api` | Webhook, job endpoints, SSE log stream | `store`, `worker` | Planned |
| `internal/config` | Environment and flag configuration | stdlib | Planned |
| `web/` | Plain HTML + JS job list and log viewer | HTTP API | Planned |

## Dependency rule

- `pipeline`, `scm`, and `store` must never import `worker` or `api`.
- `worker` talks to the `Executor` and `Store` interfaces, never to Docker or SQL directly.
- `runner` must not import Docker, SQL, git, or `net/http`; it only orchestrates through `executor.Executor`.

There is no enforcement tooling yet. Planned: a `depguard` rule or a `go list -deps` check in CI.

## Flow A: run from the CLI (Phase 3 target)

```
main
  -> parse flags, signal.NotifyContext(os.Interrupt)
  -> resolve workspace: --workspace/cwd, or scm.CloneToTemp(--repo, --sha)
  -> pipeline.ParseFile(--file)
  -> executor.NewDocker()
  -> runner.Run(ctx, pipeline, executor, Options{Workspace, Stdout, Stderr})
  -> result.WriteSummary(stdout)
  -> exit code (0 passed, 1 failed, 2 errored/infra, 130 cancelled)
```

The CLI is not wired yet; `cmd/mini-ci/main.go` only prints a placeholder.

## Flow B: from a webhook (planned, Phases 5-7)

1. GitHub sends `POST /webhook` with `X-Hub-Signature-256`.
2. `internal/api` verifies the HMAC with a constant-time compare, parses the push payload, and inserts a `queued` job.
3. The handler returns `202` immediately; no build work happens in the request.
4. A worker marks the job `running`, clones the workspace, loads the pipeline, and calls `runner.Run`.
5. Step output is appended to `log_lines` as it arrives and fanned out over SSE.
6. The worker marks the job `passed`, `failed`, or `cancelled`.

## Container model

- **One container per step** (see [DECISIONS.md](DECISIONS.md), D-002). This deviates from the original plan's "one container per pipeline".
- Container config: pipeline image, `["sh", "-c", step.Run]`, working dir `/workspace`, host workspace bind-mounted at `/workspace`, `Tty: false` so stdout/stderr can be split with `stdcopy`.
- Labels: the Docker executor always adds `ci_runner=true` and merges caller labels. The runner does not set per-job labels yet; that is needed before Phase 8 orphan cleanup.
- Image handling: inspect locally, pull if missing, fully drain the pull stream so the pull actually completes.
- Cleanup: every run defers `ContainerRemove(force, remove volumes)` on a **fresh** 15-second background context, because the job context may already be cancelled and calls made with it would fail and leak the container.

Consequence of per-step containers: files persist between steps through the bind mount, but environment variables, working directory changes, and processes do not. If that becomes a problem, switch to a per-pipeline container with `ContainerExecCreate`/`ContainerExecStart` per step; the `Executor` interface grows a session method and `runner.Run` changes, but `Result` semantics stay the same.

## Concurrency and cancellation

Today:

- `runner.Run` executes steps sequentially.
- The Docker executor starts one goroutine to copy logs and uses a buffered result channel, so an early return cannot leak the goroutine.
- Cancelling the context does **not** stop a container by itself. `ContainerWait` returns `context canceled`, then the deferred force-remove stops and deletes the container. This is covered by `internal/executor/docker_test.go` (opt-in, see below).

Planned (Phase 6):

- N worker goroutines reading from a queue; never more than N jobs running.
- One `context.Context` per job, with a timeout and an explicit cancel endpoint.
- Graceful shutdown: stop accepting jobs, then wait for or cancel in-flight ones. No job may be left `running`.
- On Windows, `SIGTERM` does not exist; `os.Interrupt` is the only portable signal. Develop under WSL2 for deployment parity.

## Data model (planned)

| Table | Columns |
|---|---|
| `jobs` | id, repo, clone_url, commit_sha, branch, status, created_at, started_at, finished_at |
| `steps` | id, job_id, name, position, status, exit_code, started_at, finished_at |
| `log_lines` | id, step_id, seq, text, created_at |

Job statuses: `queued`, `running`, `passed`, `failed`, `cancelled`. The runner also emits `skipped` and `errored` step results and uses the same status type for the overall result; the mapping to job statuses is an open decision (see D-007).

## Failure and cleanup

| Situation | Container | Workspace | Result |
|---|---|---|---|
| Step exits 0 | Removed by executor | Caller cleans up | `passed` |
| Step exits non-zero | Removed by executor | Caller cleans up | `failed`, later steps `skipped` |
| Executor/Docker error | Removed by executor | Caller cleans up | `errored`, later steps `skipped` |
| Context cancelled | Force-removed by executor | Caller cleans up | `cancelled`, later steps `skipped` |
| Image pull fails | Nothing created | Caller cleans up | `errored` |

The executor owns containers. The caller that created the workspace owns it (`scm.CloneToTemp` returns a directory whose caller must `defer os.RemoveAll`). Crash-time orphan cleanup is not built yet.

## Testing seams

- `executor.Executor` is the seam for the run loop. `internal/runner/runner_test.go` uses an in-test fake; a shared `internal/executor/fake.go` may come with Phase 6 worker tests.
- Docker integration tests are opt-in: set `MINICI_DOCKER_TEST=1`. The cancellation test waits for the container to reach `running`, cancels, asserts a `context.Canceled` error, and asserts no labelled container remains.
- The parser uses table-driven tests. `scm` was verified against a temporary local git repo; no committed test exists yet.

## Not built yet

- CLI wiring, workspace validation, temp workspace cleanup.
- Store, queue, worker pool, HTTP API, web page, config package, migrations.
- Per-job labels, orphan sweeper, resource limits, log size caps, retries, timeouts.
- Log persistence and SSE streaming; today output goes straight to the writers passed to the executor.

## Repository layout

```
cmd/mini-ci/        CLI wiring
internal/pipeline/  YAML parsing and validation
internal/executor/  Executor interface, Docker implementation, integration tests
internal/scm/       git clone at a commit
internal/runner/    step orchestration, Result and summary
docs/               this documentation set
web/                plain HTML + JS (planned)
migrations/         SQL schema (planned)
```

## How to verify the current state

```sh
go build ./...
go vet ./...
go test ./...                                                    # no Docker required
$env:MINICI_DOCKER_TEST="1"; go test ./internal/executor -run TestDocker -v
go run ./cmd/exec-try echo hello                                 # Docker smoke test
```
