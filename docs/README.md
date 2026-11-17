# Documentation

Status: Living · Scope: index of project documentation.

| Document | Purpose | Status |
|---|---|---|
| [../README.md](../README.md) | Project overview, requirements, phase task checklists | Living |
| [../PLAN.md](../PLAN.md) | Original master plan: phases, timeline, design notes | Frozen |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Component layout, flows, concurrency, failure paths | Living |
| [ROADMAP.md](ROADMAP.md) | Phase status, exit criteria, current focus | Living |
| [DECISIONS.md](DECISIONS.md) | ADR log: why things are the way they are | Living |
| [NOTES.md](NOTES.md) | Engineering log: bugs, investigations, fixes | Living |

## Conventions

- Every document declares `Status` and `Scope` at the top.
- Document only what exists; mark planned work explicitly as planned.
- One source of truth per topic: task checklists in `README.md`, phase status in `ROADMAP.md`, rationale in `DECISIONS.md`, design in `ARCHITECTURE.md`.
- ADRs are append-only. Supersede an entry instead of rewriting it.
- Update a document in the same change that makes it true or false.
