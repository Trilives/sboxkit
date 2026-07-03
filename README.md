# sboxkit

Terminal-first deployment kit for sing-box on Linux.

`sboxkit` installs and manages sing-box as a local proxy service from a TUI-first
workflow. The Debian package ships two independent binaries in one installer:
the `sboxkit` controller and the upstream `sing-box` core. The core is not linked
into this project, and this repository does not commit upstream sing-box
binaries.

## What Is Included

The `.deb` package contains:

```text
/usr/bin/sboxkit
/usr/lib/sboxkit/sing-box
/usr/share/sboxkit/base-rules/minimal.json
/lib/systemd/system/sboxkit.service
/usr/share/doc/sboxkit/copyright
/usr/share/doc/sboxkit/SING_BOX_SOURCE.txt
```

Portable `.tar.gz` bundles contain the same two binaries plus an install script.

## Install

Download the package for your architecture from GitHub Releases:

```bash
sudo apt install ./sboxkit_0.1.1_amd64.deb
```

Portable install:

```bash
tar -xzf sboxkit_0.1.1_amd64_portable.tar.gz
cd sboxkit_0.1.1_amd64
sudo ./install.sh
```

## Recommended Usage

Start the interactive terminal UI:

```bash
sboxkit
```

The TUI is the primary interface. It guides initial setup, subscription import,
TUN mode selection, optional shell proxy environment writing, service
installation, updates, node switching, and diagnostics.

For scripted usage, see [Command Reference](docs/COMMANDS.md).

## First-Run Model

Initial setup is intentionally lightweight:

1. Configure local preferences and import a subscription or local sing-box
   config.
2. Generate a working sing-box config with the built-in minimal rule file.
3. Start the service.
4. Download larger optional rule assets through the running proxy when needed.

This lets a fresh install start without requiring external rule-set downloads
before the proxy is available.

## Runtime Layout

User data is stored in a fixed state directory, independent of the current
working directory:

```text
$XDG_STATE_HOME/sboxkit/state/
~/.local/state/sboxkit/state/   # when XDG_STATE_HOME is not set
```

Override it with `SBOXKIT_ROOT=/path/to/root`, or pass `--root DIR` for one
command.

System service runtime files are staged into:

```text
/etc/sboxkit/
├── sing-box
├── sboxkit.json
├── sboxkit.cache.db
├── ruleset/
├── ui/
└── healthcheck.sh
```

## Subscription Sources

`sboxkit` can import subscriptions from remote URLs or local config files:

- `clash`: parses Clash YAML locally and converts nodes to sing-box config.
- `sing-box`: uses sing-box JSON directly, or rebuilds local policy groups when
  passthrough mode is disabled.
- `base64`: uses the configured subconverter backend first, with optional local
  Shadowsocks fallback.

Local files are copied into the fixed state directory before conversion.

## Optional WebUI

The WebUI is off by default. When enabled, sing-box serves the small WebUI built
in this repository from local assets copied into `/etc/sboxkit/ui`. It uses the
Clash API to inspect outbound groups and switch selector nodes without restarting
the service.

The WebUI is a secondary interface; the terminal UI remains the recommended way
to operate the application.

## Legal And Third-Party Assets

- `sboxkit` is built from this repository.
- `sing-box` is distributed as a separate upstream binary inside release
  packages.
- The upstream sing-box source reference is written into package documentation.
- Large third-party rule sets are downloaded only when requested by the user.
- No third-party WebUI dashboard is downloaded.

See [Third-Party Assets](docs/THIRD_PARTY_ASSETS.md) for packaging notes.

## Build From Source

```bash
go test ./...
go vet ./...
go build -o sboxkit ./cmd/sboxkit
```

The repository also supports an isolated local Go toolchain under `.tools/go`:

```bash
make test GO=.tools/go/bin/go
make vet GO=.tools/go/bin/go
make build GO=.tools/go/bin/go
```

## More Documentation

- [Command Reference](docs/COMMANDS.md)
- [Architecture](ARCHITECTURE.md)
- [Third-Party Assets](docs/THIRD_PARTY_ASSETS.md)
