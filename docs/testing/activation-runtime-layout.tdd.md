# Activation Runtime Layout TDD Evidence

## Source

User-provided target layout:

```text
/usr/bin/sboxkit
/usr/lib/sboxkit/sing-box
/usr/share/sboxkit/ui/
/etc/sboxkit/config.json
/var/lib/sboxkit/state/
/var/lib/sboxkit/activations/<revision>/
/var/lib/sboxkit/runtime -> activations/<revision>
/var/lib/sboxkit/sing-box/cache.db
/var/cache/sboxkit/downloads/
/var/cache/sboxkit/self-update/
/run/sboxkit/operation.lock
```

No compatibility layer is intended. Releases using this layout must tell users
to fully uninstall the old package/runtime state before installing.

## User Journeys

- As an operator, I want the systemd service to start only the current
  activation so service rollouts are explicit and inspectable.
- As an operator, I want `/etc/sboxkit` reserved for optional administrator
  config instead of generated runtime files.
- As a packager, I want the built-in WebUI installed under `/usr/share/sboxkit/ui`
  and copied into activations when state UI files are absent.
- As an upgrader from old releases, I want docs and package output to warn that a
  full uninstall/reinstall is required.

## RED Evidence

Targeted RED run before implementation:

```text
go test ./internal/paths ./internal/system
```

Expected failures:

```text
p.ActivationsDir undefined
p.RuntimeLink undefined
p.SingBoxCacheDB undefined
p.OperationLock undefined
p.AdminConfigFile undefined
```

## GREEN Evidence

Validation commands run after implementation:

```text
go test ./...
go vet ./...
go build -o /tmp/sboxkit-check ./cmd/sboxkit
git diff --check
```

Packaging smoke checks also passed:

```text
packaging/deb/build-deb.sh --version 0.2.0 --arch amd64 ...
packaging/portable/build-portable.sh --version 0.2.0 --arch amd64 ...
```

The generated Debian package contains `/usr/share/sboxkit/ui/index.html`, and
its packaged `sboxkit.service` starts:

```text
/var/lib/sboxkit/runtime/bin/sing-box run -c /var/lib/sboxkit/runtime/config.json
```

## Guarantees

| # | Guarantee | Test or command | Result |
|---|---|---|---|
| 1 | Default root paths expose state, activations, runtime symlink, cache DB, cache downloads, admin config, and operation lock | `internal/paths/paths_test.go` | PASS |
| 2 | Rendered systemd unit starts from `runtime/bin/sing-box` and `runtime/config.json` | `internal/system/service_test.go` | PASS |
| 3 | Service sync stages core/config/rules/UI into an activation and switches `runtime` | `internal/system/service_test.go` | PASS |
| 4 | WebUI status reports the runtime symlink path | `internal/webui/server_test.go` | PASS |
| 5 | Diagnostics list new state/cache/runtime/admin paths | `internal/app/tui_menu_test.go` | PASS |
| 6 | Debian and portable packaging include the built-in WebUI under share/ui or bundle/ui | packaging smoke commands | PASS |

