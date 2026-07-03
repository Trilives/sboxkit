# TDD Evidence: TUI Menu Flow

Source plan: derived from the interactive requirements in this session.

User journeys:
- As a terminal user, I want the main menu grouped into six high-level actions, so common workflows are easier to reach.
- As a first-time user, I want language selection before initialization, so the wizard starts in the preferred language.
- As a subscription user, I want switching subscriptions to rebuild the active config, so stale generated config is not applied.
- As a config editor user, I want Esc to cancel/return instead of saving, so interrupted edits do not persist accidentally.

Task report:
- RED: `GOCACHE=/tmp/sboxkit-gocache GOMODCACHE=/home/ares/Workspace/Singbox/.tools/go-mod .tools/go/bin/go test ./internal/app ./internal/system ./internal/tui` failed with missing service detection/menu functions, missing `Service.Start`/`Stop`, and Esc mapped to save.
- GREEN: the same command passed after implementing the six-item menu (`No-Restart Changes`, `Restart Required`, `Diagnostics`, `Service Control`, `Language / 语言`, `Uninstall`), auto first-setup gate, language-first setup, service start/stop commands, switch-and-rebuild behavior, and Esc cancellation.
- Full verification: `GOCACHE=/tmp/sboxkit-gocache GOMODCACHE=/home/ares/Workspace/Singbox/.tools/go-mod .tools/go/bin/go test ./...` passed.
- Build verification: `GOCACHE=/tmp/sboxkit-gocache GOMODCACHE=/home/ares/Workspace/Singbox/.tools/go-mod .tools/go/bin/go build -o /tmp/sboxkit-check ./cmd/sboxkit` passed.

Guarantees:

| # | What is guaranteed | Test file or command | Type | Result |
|---|---|---|---|---|
| 1 | Main TUI has exactly six top-level items in English and Chinese | `internal/app/tui_test.go` | Unit | PASS |
| 2 | Missing service integration triggers first setup before the main menu, and setup asks language first | `internal/app/tui_test.go` | Unit | PASS |
| 3 | A stopped unit file still counts as existing service integration | `internal/app/tui_test.go` | Unit | PASS |
| 4 | `sub switch` rebuilds stale subscription config before activating it | `internal/app/app_test.go` | Integration | PASS |
| 5 | Service start/stop issue the expected `systemctl` commands | `internal/system/service_test.go` | Unit | PASS |
| 6 | Esc cancels selection and Ctrl+R is the explicit save-exit shortcut | `internal/tui/tui_test.go` | Unit | PASS |

Coverage:
- `GOCACHE=/tmp/sboxkit-gocache GOMODCACHE=/home/ares/Workspace/Singbox/.tools/go-mod .tools/go/bin/go test -cover ./...` passed.
- Existing package coverage remains below 80% in several packages; this task added focused regression coverage but did not attempt a repository-wide coverage lift.
