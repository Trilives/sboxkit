# TDD Evidence: Node Switch Order And Prompt Wrapping

Source plan: derived from the interactive requirements in this session.

User journeys:
- As a terminal user, I want long input prompts to wrap, so prompts do not push the input field off-screen.
- As a node-switching user, I want runtime node switching to avoid changing generated node order by default.
- As a node-switching user, I want an explicit option to move the selected node first in generated config, with service sync/restart only when I choose it.

Task report:
- RED: `go test ./internal/node ./internal/app ./internal/tui` failed because `ReorderSelectorConfig`, `buildSwitchNodeArgs`, and input prompt wrapping fields did not exist.
- GREEN: `go test ./internal/node ./internal/app ./internal/tui ./internal/updater` passed after adding selector reorder logic, default no-reorder TUI behavior, and wrapped input prompts.
- Full verification: `go test ./...` passed.
- Build verification: `go build -o /tmp/sboxkit-check ./cmd/sboxkit` passed.

Guarantees:

| # | What is guaranteed | Test file or command | Type | Result |
|---|---|---|---|---|
| 1 | Long TUI input prompts wrap within terminal width | `internal/tui/tui_test.go` | Unit | PASS |
| 2 | Generated selector order moves the selected node first only when requested | `internal/node/order_test.go` | Unit | PASS |
| 3 | TUI node switching does not request reorder by default | `internal/app/node_tui_test.go` | Unit | PASS |
| 4 | TUI node switching can request reorder and service sync explicitly | `internal/app/node_tui_test.go` | Unit | PASS |

Coverage:
- `go test -cover ./...` passed.
- Existing repository-wide coverage remains below 80% in multiple packages; this task added focused regression coverage.
