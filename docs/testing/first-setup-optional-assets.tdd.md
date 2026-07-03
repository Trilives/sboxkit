# TDD Evidence: First Setup Optional Asset Failure

Source plan: derived from the interactive requirement in this session.

User journey:
- As a first-time user, I want service startup to remain successful even if optional post-start rule downloads fail, so a nested optional step does not break the already running proxy service.

Task report:
- RED: `go test ./internal/app` failed because first setup still hard-called the update path and treated optional rule download failure as a setup failure.
- GREEN: `go test ./internal/app` passed after moving service/update execution behind session-level call points and making the post-start rule update non-fatal.
- Full verification: `go test ./...` passed.
- Build verification: `go build -o /tmp/sboxkit-check ./cmd/sboxkit` passed.

Guarantees:

| # | What is guaranteed | Test file or command | Type | Result |
|---|---|---|---|---|
| 1 | First setup starts the service, attempts optional rules, and returns success when optional rules fail | `internal/app/tui_test.go` | Unit | PASS |
| 2 | The user sees a warning explaining the service remains running and assets can be retried later | `internal/app/tui_test.go` | Unit | PASS |

Coverage:
- `go test -cover ./...` passed.
- Existing repository-wide coverage remains below 80% in multiple packages; this task added focused regression coverage for the requested failure mode.
