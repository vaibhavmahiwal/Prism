# Decisions

Status: Living · Scope: architecture decision records (ADRs).

Append-only. To change a decision, add a new entry that supersedes the old one; do not rewrite history.

Entry format: id, date, status, context, decision, consequences.

---

## D-001: Keep Docker behind an `Executor` interface

- **Date:** 2026-11-17
- **Status:** Accepted
- **Context:** The run loop must be testable without Docker, and Docker may later be swapped for Kubernetes. The worker pool should not import the Docker SDK.
- **Decision:** Define `Executor.Run(ctx, Spec, stdout, stderr) (exitCode int, err error)` with `Spec{Image, Command, WorkDir, Env, Labels, Mounts}`. The Docker implementation lives in `internal/executor/docker.go`; callers depend only on the interface.
- **Consequences:** The run loop is unit-tested with a fake. Docker SDK types stay out of `runner` and `worker`. One `Run` call creates one container, which shaped D-002. A per-pipeline container model would need a new session method on the interface.

---

## D-002: One container per step

- **Date:** 2026-11-17
- **Status:** Accepted (documented deviation from PLAN section 6)
- **Context:** PLAN's design table prefers one container per pipeline with steps running inside it. The current `Executor.Run` starts a container per call, and the simplest correct Phase 3 implementation uses it once per step.
- **Decision:** Run each pipeline step in its own container, bind-mounting the host workspace at `/workspace`.
- **Consequences:** Per-step exit codes, durations, and logs map directly to the `steps` table. Files persist through the mount; environment variables, working directory changes, and background processes do not. Each step pays container startup cost. Revisit if step state must carry over; the fallback is `ContainerExecCreate`/`ContainerExecStart` inside one container.

---

## D-003: Full clone, then detached checkout

- **Date:** 2026-11-17
- **Status:** Accepted
- **Context:** Webhook jobs specify a commit SHA; the workspace must be exactly that commit. Fetching an arbitrary SHA is not supported by every git host, and shallow clones may not contain it.
- **Decision:** `git clone --quiet --no-checkout <url> <dest>`, then `git checkout --detach <sha>`. Validate the SHA as 7-40 hex characters before invoking git. Set `GIT_TERMINAL_PROMPT=0` so a server-side run never blocks on credentials. `CloneToTemp` returns a temp directory; the caller removes it.
- **Consequences:** Correct for any commit reachable from a ref, at the cost of downloading full history. No auth support yet. Repository URLs with embedded credentials may appear in git's error output; do not pass tokens this way.

---

## D-004: Runner error semantics

- **Date:** 2026-11-17
- **Status:** Accepted
- **Context:** Callers need to distinguish "the build failed" from "we could not run the build". Collapsing both into an error makes exit codes and status mapping ambiguous.
- **Decision:** `runner.Run` returns a non-nil error only when it cannot execute at all (nil pipeline or executor). Everything else is recorded in `Result` and `StepResult` with step statuses `passed`, `failed`, `errored`, `cancelled`, or `skipped`. Overall status precedence: `cancelled` > `errored` > `failed` > `passed`; skipped steps never change the overall status. Execution stops at the first non-passed step; later steps are marked `skipped`.
- **Consequences:** The CLI maps `Result.Status` to process exit codes; the store maps it to job statuses later. `skipped` and `errored` are not job statuses, so a mapping decision is still open (D-007).

---

## D-005: Docker integration tests are opt-in

- **Date:** 2026-11-17
- **Status:** Accepted
- **Context:** `go test ./...` must work on machines without Docker, and integration tests that create containers are slower and environment-dependent.
- **Decision:** Skip Docker tests unless `MINICI_DOCKER_TEST=1` is set. Keep unit tests on fakes. The cancellation test waits for `State == "running"`, cancels, asserts a `context.Canceled` error, and asserts that no labelled container remains.
- **Consequences:** CI without Docker still has green unit tests. Integration coverage must be run deliberately; Phase 9 should add it to the documented test commands.

---

## D-006: Label every container, and export `Spec.Labels`

- **Date:** 2026-11-17
- **Status:** Accepted
- **Context:** Phase 8 needs safe orphan cleanup, and tests need to find only their own containers.
- **Decision:** The Docker executor always applies `ci_runner=true` and merges caller-provided `Spec.Labels`. `Labels` is exported so callers and tests can tag containers.
- **Consequences:** Orphan sweeps can filter on `ci_runner=true`. Job/step identity labels are not set by `runner` yet; that should be added before or during Phase 6/8.

---

## D-007: Job status mapping

- **Date:** 2026-11-17
- **Status:** Proposed — decide in Phase 4
- **Context:** The runner has five step statuses, the job model has five values: `queued`, `running`, `passed`, `failed`, `cancelled`.
- **Decision (proposal):** Map `failed` and `errored` to job status `failed`, store the failure reason on the job, and keep `skipped` only at the step level. Do not add `errored` as a job status.
- **Consequences:** The API stays simple; errors are still visible in step rows and in the job's failure reason. Alternative: add an `errored` job status, which complicates every client.

---

## D-008: CLI exit-code scheme

- **Date:** 2026-11-17
- **Status:** Accepted — implemented in `cmd/mini-ci/main.go`, covered by `main_test.go`
- **Context:** Scripts and humans need stable codes.
- **Decision (proposal):** `0` all steps passed, `1` a step exited non-zero, `2` infrastructure or configuration error, `130` interrupted (128 + SIGINT). Bad flags and invalid workspaces are `2`.
- **Consequences:** Failed builds are distinguishable from a broken runner. The runner itself never exits; `main` owns the mapping.

---

## Open decisions

- Queue mechanism: buffered channel vs polling the database for `queued` jobs.
- Webhook scope: which events create jobs, branch filtering, duplicate-delivery handling.
- Timeout scope: per job, per step, or both, and when the clock starts.
- Log transport: direct writers today; per-step writers plus SSE later; ordering and truncation guarantees.
- Shallow clone or fetch-by-SHA optimization once clones become slow.
- Migration tooling and Postgres support behind the same `Store` interface.
