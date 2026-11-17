## Runner decisions

- `runner.Run` returns a non-nil `error` only when execution cannot be started because its pipeline or executor argument is nil. Step-level command failures and executor/infrastructure failures are recorded in `Result.Steps` and do not use the function error.
- The active step's outcome determines the overall status: cancellation wins when the context is cancelled, otherwise an executor error wins over a non-zero exit, which wins over success. Skipped steps do not change the overall status.
- The runner writes a header before each step and exposes `Result.WriteSummary` for a final tabular summary containing name, status, and duration.

