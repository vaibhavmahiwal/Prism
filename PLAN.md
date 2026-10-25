# mini-ci: Project Plan

A small self-hosted CI runner written in Go. A GitHub webhook hits the server, the server queues a job, a worker runs the steps from a YAML pipeline file inside Docker containers, and logs and status are available through an API and a simple web page.

**Learning goals:** concurrency (goroutines, channels, `context`), driving Docker from Go, webhook security, streaming, durable state, graceful shutdown, packaging.

**Out of scope:** multi-node runners, a frontend framework, secrets management, deployment to servers. These are stretch goals.

---

## 1. How it works

```
GitHub --webhook--> HTTP server --> job queue --> worker pool
                        |                             |
                        v                             v
                   SQLite/Postgres               Docker Engine
                   (jobs, steps, logs)        (one container per step)
                        ^                             |
                        '------ live logs <-----------'
```

Example pipeline file (`.mini-ci.yml`) in a repo being built:

```yaml
name: build-and-test
image: golang:1.23
steps:
  - name: test
    run: go test ./...
  - name: build
    run: go build ./...
```

---

## 2. Repository structure

```
mini-ci/
├── cmd/
│   └── mini-ci/
│       └── main.go              # wiring only: config, store, queue, workers, server, shutdown
├── internal/
│   ├── pipeline/                # Phase 1: YAML -> validated Go structs (pure logic)
│   │   ├── types.go
│   │   ├── parser.go
│   │   └── parser_test.go
│   ├── executor/                # Phases 2-3: run a step; interface + implementations
│   │   ├── executor.go          # Executor interface
│   │   ├── docker.go            # real implementation (Docker Engine SDK)
│   │   ├── fake.go              # test double, no Docker needed
│   │   └── docker_test.go
│   ├── scm/                     # clone a repo at a commit, verify webhook signatures
│   │   ├── git.go
│   │   └── signature.go
│   ├── store/                   # Phase 4: persistence behind an interface
│   │   ├── store.go             # Store interface + model structs
│   │   ├── sqlite.go
│   │   └── store_test.go
│   ├── queue/                   # Phase 6: job queue
│   │   └── queue.go
│   ├── worker/                  # Phase 6: worker pool, cancellation, shutdown
│   │   ├── pool.go
│   │   └── pool_test.go
│   ├── api/                     # Phases 5 and 7: HTTP layer
│   │   ├── server.go
│   │   ├── webhook.go
│   │   ├── jobs.go
│   │   └── logs.go              # streaming (SSE)
│   └── config/
│       └── config.go
├── migrations/
│   └── 001_init.sql
├── web/
│   └── index.html               # plain HTML + a little JS, polls the API
├── docs/
│   └── NOTES.md                 # running log: bugs hit, how found, how fixed
├── .mini-ci.yml                 # mini-ci building itself (Phase 9)
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod / go.sum
└── PLAN.md
```

**Dependency rule:** `pipeline`, `scm`, and `store` never import `worker` or `api`. `worker` talks to the `Executor` and `Store` interfaces, never to Docker or SQL directly. This is what makes the worker pool testable without Docker or a database, the same idea as the `EngineProfile` interface in Mini-Everest.

---

## 3. Data model

| Table | Columns |
|---|---|
| `jobs` | id, repo, clone_url, commit_sha, branch, status, created_at, started_at, finished_at |
| `steps` | id, job_id, name, position, status, exit_code, started_at, finished_at |
| `log_lines` | id, step_id, seq, text, created_at |

Job status values: `queued`, `running`, `passed`, `failed`, `cancelled`.

---

## 4. Phases

### Phase 0: Prerequisites (3-4 days, if these are new)
- Go: goroutines, channels, `sync.WaitGroup`, `context.Context`, `io.Reader`/`io.Writer`.
- Docker: images vs containers, `docker run`, volumes, exit codes.
- HTTP: `net/http` handlers, JSON encoding, what a webhook is.

**Done when:** you can write a program that starts 5 goroutines, collects results over a channel, and cancels them all with a context timeout.

### Phase 1: Pipeline parser
**Goal:** read a pipeline YAML into structs and validate it. No Docker, no server.
- Define `Pipeline` and `Step` structs with YAML tags.
- Validate: image required, at least one step, step names unique and non-empty.
- Return clear error messages that name the offending field.

**Done when:** table-driven tests cover a valid file, a missing image, an empty steps list, and duplicate step names.

### Phase 2: Run one command in one container
**Goal:** the smallest useful Docker integration.
- Define the `Executor` interface, for example `Run(ctx, spec) (exitCode int, err error)`.
- Implement it with the Docker Engine Go SDK: pull the image, create the container, start it, wait, read the exit code, remove the container.
- Print the container output to your terminal.

**Done when:** a throwaway `main` runs `echo hello` in `alpine` and prints `hello`, and `docker ps -a` shows no leftover container.

### Phase 3: Run a whole pipeline from the command line
**Goal:** `mini-ci run .mini-ci.yml` works with no server.
- Clone the repo (or use a local directory) and mount it into the container as the workspace.
- Run steps in order, stop at the first non-zero exit code, and report which step failed.
- Clean up the container and temp workspace on every path: success, failure, and Ctrl+C.

**Done when:** a passing pipeline exits 0, a failing one exits 1 and names the failing step, and Ctrl+C leaves nothing running.

### Phase 4: Store and job model
**Goal:** durable state behind an interface.
- Write `001_init.sql` and a `Store` interface: create job, get job, list jobs, update status, append log lines, list log lines.
- Implement it with SQLite first. Postgres can come later through the same interface.

**Done when:** store tests pass against a temporary database, including status transitions.

### Phase 5: HTTP server and webhook
**Goal:** a push event creates a queued job.
- `POST /webhook` verifies the `X-Hub-Signature-256` HMAC using a constant-time compare and rejects bad signatures with 401.
- Parse the push payload (repo, clone URL, SHA, branch), insert a `queued` job, and return 202 quickly.
- `GET /healthz` returns OK.

**Done when:** a signed test payload sent with `curl` creates a job row, an unsigned or tampered one is rejected, and the handler returns without running anything.

### Phase 6: Queue, workers, cancellation, shutdown
**Goal:** jobs run in the background, safely.
- A queue (buffered channel, or poll the DB for `queued` jobs) feeding N worker goroutines.
- Each worker: mark `running`, run the pipeline through the `Executor`, record step results, mark `passed` or `failed`.
- Pass a `context` through every layer. Add a per-job timeout and a cancel path that actually stops the container, because cancelling a Go context does not stop a Docker container by itself.
- Graceful shutdown: on SIGTERM stop taking jobs, then wait for or cancel in-flight ones.

**Done when:** the pool test using the fake executor shows N jobs running concurrently and never more than N, a cancelled job ends as `cancelled`, and shutdown leaves no job stuck in `running`.

### Phase 7: Logs and the API
**Goal:** see what happened, live.
- Copy container output line by line into `log_lines` as it arrives.
- `GET /jobs`, `GET /jobs/{id}`, `POST /jobs/{id}/cancel`.
- `GET /jobs/{id}/logs` streams new lines live using server-sent events, and replays stored lines first.
- `web/index.html` lists jobs and shows logs by polling or using the SSE endpoint.

**Done when:** you push a commit (or send a test webhook), open the page, and watch the logs appear while the job runs.

### Phase 8: Reliability
**Goal:** it behaves under failure.
- **Crash recovery:** on startup, any job left in `running` is re-queued or marked failed with a clear reason.
- **Orphan cleanup:** label every container the runner creates, and remove stale labeled containers at startup.
- **Resource limits:** set CPU and memory limits and a maximum log size per step.
- **Retries:** decide what happens when the clone fails or Docker is unreachable, and record the reason on the job.

**Done when:** killing the server mid-job with `kill -9`, then restarting it, leaves no stuck job and no orphan container.

### Phase 9: Packaging, tests, docs
**Goal:** someone else can run it.
- Multi-stage `Dockerfile` and a `docker-compose.yml` that starts the server and mounts the Docker socket.
- `Makefile` targets: `build`, `test`, `lint`, `run`.
- Add a `.mini-ci.yml` so mini-ci builds and tests itself, which is a good demo.
- Write the README: install, configure the webhook, security caveats, a short terminal recording.
- Cover coverage gaps: parser, store, worker pool with the fake executor, webhook signature handling.

**Done when:** `docker compose up` on a clean machine runs it, `go test ./...` passes, and mini-ci's own pipeline passes on its own repo.

### Phase 10: Stretch goals (pick any)
- Run steps as Kubernetes Jobs through a second `Executor` implementation (reuses your Mini-Everest knowledge).
- Prometheus metrics: queue depth, job duration, failure rate.
- Per-job environment variables and a simple secrets mechanism.
- Cache directories between runs.
- Post status back to GitHub as a commit status check.

---

## 5. Timeline (part-time, rough)

| Week | Phases |
|---|---|
| 1 | 0, 1, 2 |
| 2 | 3, 4, 5 |
| 3 | 6, 7 |
| 4 | 8, 9 |
| 5+ | 10 |

---

## 6. Design decisions to document in `docs/NOTES.md`

| Decision | Reason |
|---|---|
| `Executor` and `Store` are interfaces | Test the worker pool with fakes, and swap Docker for Kubernetes or SQLite for Postgres later |
| Webhook returns 202 immediately | Builds take minutes and GitHub expects a fast reply |
| One container per pipeline, steps run inside it | Simple to start. Per-step containers are an easy change later |
| SQLite first | No extra service to run while learning, same SQL interface |
| Label every container the runner creates | Makes orphan cleanup safe |

## 7. Known gaps to state openly

- **No sandboxing.** It runs arbitrary code from repos. Run it locally or on a throwaway machine, never expose it publicly, and note this in the README.
- Mounting the Docker socket gives the runner (and anything in its container) root-equivalent access to the host.
- Single node only, no horizontal scaling.
- No authentication on the web page or API beyond the webhook signature.
- No secrets management.

## 8. Common pitfalls

- Leaked containers and temp directories on cancel and shutdown paths.
- Forgetting that cancelling a context does not stop a container.
- Running the build inside the webhook handler.
- Testing the webhook locally: GitHub cannot reach your laptop, so use `curl` with a signed payload, or a tunnel like ngrok or smee.io.
- Reading container logs without handling the multiplexed stdout/stderr stream format.

## 9. Resume line (use only what is built and tested)

"Built a self-hosted CI runner in Go with an HMAC-verified webhook receiver, a concurrent worker pool executing pipeline steps in Docker containers, live log streaming, and crash-safe job state."

Keep the "(in progress)" label until Phase 8 passes.
