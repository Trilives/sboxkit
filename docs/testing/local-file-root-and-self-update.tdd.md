# Local File Root and Self-Update TDD Evidence

## Source

Derived from the confirmed implementation plan based on the later
`mihomo-cli-deploy` history.

## User Journeys

- As an operator, I want `sboxkit` state to use `/var/lib/sboxkit` by default so
  user sessions and systemd see the same data.
- As an operator, I want local YAML/JSON files imported through the converter so
  local files behave consistently and do not passthrough sing-box JSON.
- As an operator, I want local-file subscription import integrated into "Add
  Subscription", with a separate overwrite action for replacing the current
  configuration.
- As an operator, I want stable and preview self-update channels.

## RED Evidence

Initial targeted run:

```text
go test ./internal/paths ./internal/app ./internal/updater
```

Expected failures before implementation:

```text
undefined: Channel
manager.CheckChannel undefined
session.buildAddSubscriptionArgs undefined
unexpected default root ".../sboxkit"
```

Overwrite-specific RED:

```text
TestSubOverwriteLocalCanReplaceExistingLocalSlot:
unknown sub command: overwrite-local
```

## GREEN Evidence

Validation commands run after implementation:

```text
go test ./...
go vet ./...
go build -o /tmp/sboxkit-check ./cmd/sboxkit
git diff --check
```

All commands passed.

Coverage command also passed:

```text
go test ./... -cover
```

The repository still has pre-existing package-level coverage below 80% in
several packages. This change added focused tests for the changed behavior but
did not attempt a repository-wide coverage remediation.

## Guarantees

| # | Guarantee | Test file | Result |
|---|---|---|---|
| 1 | Default root is `/var/lib/sboxkit` unless `SBOXKIT_ROOT` is set | `internal/paths/paths_test.go` | PASS |
| 2 | Local sing-box JSON files are converted even if `--passthrough` is supplied | `internal/app/app_test.go` | PASS |
| 3 | Local overwrite replaces the existing `local-overwrite` slot and activates it | `internal/app/app_test.go` | PASS |
| 4 | Add Subscription accepts `local-file` as a source without showing passthrough prompts | `internal/app/tui_menu_test.go` | PASS |
| 5 | Subscription menu exposes local-file overwrite separately but does not list a separate local add item | `internal/app/tui_menu_test.go` | PASS |
| 6 | Update menu exposes stable and preview self-update channels | `internal/app/tui_menu_test.go` | PASS |
| 7 | Updater checks preview releases through the preview channel | `internal/updater/updater_test.go` | PASS |

