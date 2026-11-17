# Roadmap

Status: Living · Scope: phase status, exit criteria, current focus.

Task-level checklists live in [../README.md](../README.md). Design rationale lives in [DECISIONS.md](DECISIONS.md). This document tracks status only.

Last updated: 2026-11-17

## Snapshot

- **Current phase:** 3 — run a pipeline from the CLI.
- **Progress:** roughly 25-30% of the core build (Phases 1-9).
- **Tests:** `go test ./...` passes; Docker integration test is opt-in.
- **Next milestone:** `mini-ci run .mini-ci.yml` works end to end.

## Phase status

| Phase | Status | Exit criteria met | Notes |
|---|---|---|---|
| 0. Prerequisites | Done | Assumed | Go 1.26, Docker 28, git 2.50 present |
| 1. Pipeline parser | Done | Yes | Parse, strict unknown fields, validation, tests |
| 2. One command in one container | Done | Yes | `Executor` + Docker impl, log splitting, cancellation cleanup verified |
| 3. Pipeline from the CLI | In progress | No | `scm` and `runner` done; CLI is still a stub |
| 4. Store and job model | Not started | No | SQLite first |
| 5. HTTP server and webhook | Not started | No | HMAC, 202 response |
| 6. Queue, workers, shutdown | Not started | No | Context, timeout, graceful shutdown |
| 7. Logs and API | Not started | No | SSE, job endpoints, web page |
| 8. Reliability | Not started | No | Crash recovery, orphans, limits, retries |
| 9. Packaging and docs | Not started | No | Dockerfile, compose, Makefile, dogfooding |
| 10. Stretch goals | Not started | No | Kubernetes executor, metrics, caching, status checks |

## Phase 3: remaining work

1. Wire `cmd/mini-ci/main.go`: `run` subcommand, flags, usage, exit-code mapping.
2. Resolve workspace: local cwd or `scm.CloneToTemp`; validate absolute/existing directory.
3. Add `signal.NotifyContext` and guarantee temp workspace cleanup on success, failure, cancel.
4. Verify: pass exits 0, failure exits 1 and names the step, Ctrl+C exits 130 with no `ci_runner` containers left.

Hygiene before marking Phase 3 done:

- `.mini-ci.yml` still pins `golang:1.23` while `go.mod` is 1.26.
- `cmd/mini-ci/mini-ci.exe` is committed; remove it from git and add `.gitignore`.
- Add container labels for job/step identity (needed in Phase 8, nice to have now).

## Exit criteria by phase

| Phase | Done when |
|---|---|
| 1 | Tests cover valid file, missing image, empty steps, empty/duplicate names |
| 2 | `echo hello` in `alpine` prints output and leaves no container |
| 3 | Passing pipeline exits 0; failing exits 1 and names the step; Ctrl+C leaves nothing running |
| 4 | Store tests pass against a temp DB, including status transitions |
| 5 | Signed payload creates a queued job; bad signature is 401; handler returns immediately |
| 6 | Fake-executor test shows N concurrent jobs, never more; cancel ends `cancelled`; shutdown leaves no `running` job |
| 7 | Test webhook then watch logs live in the browser |
| 8 | `kill -9` mid-job then restart leaves no stuck job and no orphan container |
| 9 | `docker compose up` on a clean machine works; `go test ./...` passes; self-pipeline passes |
| 10 | Whatever stretch goal is picked is demonstrably working |

## Risks and open items

- Job status mapping is unresolved: the runner produces `errored`/`skipped`, the job model only has `queued`/`running`/`passed`/`failed`/`cancelled` (D-007).
- Per-step containers mean environment state does not carry across steps (D-002).
- Windows lacks `SIGTERM`, and bind-mount paths differ from Linux; WSL2 is recommended.
- Log streaming design is unresolved: direct writers today, per-step attributed writers plus SSE later.

## Stretch goals

- Kubernetes Job executor as a second `Executor`.
- Prometheus metrics: queue depth, job duration, failure rate.
- Per-job environment and a minimal secrets mechanism.
- Cache directories between runs.
- Commit status checks posted back to GitHub.
