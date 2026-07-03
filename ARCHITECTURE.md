# sboxkit Architecture

`sboxkit` is a terminal-first Go application for Linux systems running systemd. Releases are
published as Debian packages and portable tarballs. Each release artifact
aggregates two independent executables:

- `sboxkit`, the primary TUI/CLI manager built from this repository.
- `sing-box`, downloaded from upstream release assets during the release job.

The sing-box source is not linked into the manager binary, and the repository
does not commit upstream core binaries.

## Package Layout

```text
cmd/sboxkit/        binary entrypoint
internal/app/           command dispatch and interactive menu
internal/config/        customize.json model, defaults, typed updates
internal/converter/     Clash/base64/sing-box to sing-box config conversion
internal/download/      GitHub release lookup, asset download, archive extraction
internal/nettest/       latency probes through local proxy when available
internal/node/          Clash API node listing and switching
internal/paths/         state/runtime path layout
internal/subscription/  fetch, detect, convert, persist, switch subscriptions
internal/system/        runner abstraction, systemd, timers, firewall, resilience
internal/testkit/       golden fixture helpers
internal/transaction/   rollback primitives
internal/version/       build metadata
internal/webui/         optional embedded WebUI and local status API
testdata/               golden fixtures
```

## State Layout

User state uses a stable root and never follows the shell's current working
directory. Resolution order:

1. `SBOXKIT_ROOT`
2. `$XDG_STATE_HOME/sboxkit`
3. `~/.local/state/sboxkit`

State layout under that root:

```text
state/
├── active
├── config.json
├── customize.json
├── bin/
├── downloads/
├── ruleset/
├── subscriptions/
└── ui/
```

Installed system runtime:

```text
/etc/sboxkit/
├── sing-box
├── sboxkit.json
├── sboxkit.cache.db
├── ruleset/
├── ui/
└── healthcheck.sh
```

## Design Boundaries

- All system mutations go through `internal/system.Runner`, so tests can use fake
  runners instead of executing `systemctl`, `install`, or firewall commands.
- Subscription conversion is local by default for Clash and sing-box sources.
- Base64 uses a subconverter backend first because arbitrary share-link parsing is
  broad and error-prone; local fallback is intentionally limited.
- Release artifacts may include the upstream sing-box core as a separate
  executable. Web UI files, rule sets, subscriptions, and subconverter software
  are kept out of the repository and Debian package.
- The Debian package installs `/usr/bin/sboxkit`, the minimal
  bootstrap rules at `/usr/share/sboxkit/base-rules/minimal.json`, a packaged
  `sboxkit.service`, and the independent upstream
  core at `/usr/lib/sboxkit/sing-box`.
- Service installation copies the selected core into `/etc/sboxkit/sing-box`.
  A user-updated `state/bin/sing-box` takes precedence over the packaged core.
- Web UI files, rule sets, subscriptions, and subconverter software are not
  bundled. They remain runtime downloads governed by their upstream licenses.
- The managed service is `sboxkit.service`, with runtime files under
  `/etc/sboxkit`.

## First-Run Bootstrap

The runtime deliberately has two phases:

1. Generate and run a small configuration from the user's subscription. Before
   large `.srs` assets exist, generated configs use built-in domain/IP rules,
   documented by the package's minimal bootstrap rules, and omit local
   `rule_set` references so `sing-box check` and first service start do not
   depend on external rule downloads.
2. After `sboxkit.service` is running, run `sboxkit update --proxy
   http://127.0.0.1:7890 --sync-service`. This downloads large rule-set assets
   through the running proxy, rebuilds the active
   subscription config so local `rule_set` entries are enabled, copies the
   assets into `/etc/sboxkit`, checks the runtime config, and restarts the
   service.

The sing-box WebUI is project-owned and optional. When `lan_panel` is enabled,
`sboxkit` writes embedded UI files into `state/ui`, generated configs set
`experimental.clash_api.external_ui`, and service sync copies those files to
`/etc/sboxkit/ui`. The UI uses same-origin Clash API calls for runtime status
and selector switching; it does not download third-party dashboards.

## Verification

Primary local checks:

```bash
go test ./...
go vet ./...
go build ./cmd/sboxkit
```

The CI workflow runs test, vet, and build on pull requests and pushes to `main`.
The release workflow builds Debian packages, portable tarballs, and checksums on
`v*` tags.
