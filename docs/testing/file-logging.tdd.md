# TDD Evidence: File Logging

Source plan: derived from the logging requirement in this session.

User journeys:
- As a user, I want file logging to be configurable, so normal runs do not write logs unless I enable them.
- As an operator, I want logs saved under the fixed state directory, so I can inspect failures regardless of current working directory.
- As an operator, I want old logs deleted automatically, so logs cannot grow without bound.

Task report:
- RED: `go test ./internal/applog` failed because `Open`, `Prune`, and size limit helpers did not exist.
- GREEN: `go test ./internal/applog ./internal/config ./internal/paths ./internal/app` passed after adding the log package, config fields, log path layout, and command-entry logging.
- Full verification: `go test ./...` passed.
- Build verification: `go build -o /tmp/sboxkit-check ./cmd/sboxkit` passed.

Guarantees:

| # | What is guaranteed | Test file or command | Type | Result |
|---|---|---|---|---|
| 1 | Disabled logging does not create a log directory | `internal/applog/applog_test.go` | Unit | PASS |
| 2 | Enabled logging mirrors stderr to a log file | `internal/applog/applog_test.go` | Unit | PASS |
| 3 | Old log files are deleted when total size exceeds the configured cap | `internal/applog/applog_test.go` | Unit | PASS |
| 4 | Log max size is clamped with a hard 100 MB cap | `internal/applog/applog_test.go` | Unit | PASS |
| 5 | `enable_file_log` and `log_max_mb` load, save, and update through config fields | `internal/config/config_test.go` | Unit | PASS |
| 6 | Commands write start, stderr, and exit code to file logs when enabled | `internal/app/app_test.go` | Integration | PASS |

Coverage:
- `go test -cover ./...` passed.
- Existing repository-wide coverage remains below 80% in multiple packages; this task added focused logging coverage.
