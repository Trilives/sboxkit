# Command Reference

The recommended way to use `sboxkit` is the interactive terminal UI:

```bash
sboxkit
```

Use the commands below for automation, remote maintenance, or recovery tasks.

## Help And Version

```bash
sboxkit help
sboxkit version
```

## Initial Setup

Interactive setup is preferred:

```bash
sboxkit
```

Non-interactive setup:

```bash
sboxkit init
sboxkit init --download --proxy http://127.0.0.1:7890
sboxkit init --no-tun
sboxkit init --no-tun --write-proxy-env
```

When TUN is disabled in the TUI, `sboxkit` asks whether it should write shell
proxy variables to `~/.bashrc`.

## Subscriptions And Local Configs

Add a remote Clash subscription:

```bash
sboxkit sub add --name main --source clash --url "https://example.com/sub.yaml"
```

Add a local config file:

```bash
sboxkit sub add --name local --file ~/config.yaml
sboxkit sub add --name direct --file ~/config.json --source sing-box
sboxkit sub add --name direct --file ~/config.json --source sing-box --passthrough
```

Manage subscriptions:

```bash
sboxkit sub list
sboxkit sub switch --name NAME
sboxkit sub refresh --name NAME
sboxkit sub refresh --name NAME --proxy http://127.0.0.1:7890
sboxkit sub rebuild --name NAME
sboxkit sub remove --name NAME
```

## Service Management

Install and start the systemd service:

```bash
sudo sboxkit service install
```

Other service commands:

```bash
sudo sboxkit service install --no-start
sudo sboxkit service sync
sudo sboxkit service status
sudo sboxkit service remove
```

## Runtime Asset Updates

After the service is running, download optional runtime assets through the proxy
and sync them into `/etc/sboxkit`:

```bash
sboxkit update --proxy http://127.0.0.1:7890 --sync-service
```

Other update forms:

```bash
sboxkit update
sboxkit update --force
sboxkit update --core
```

## Configuration

```bash
sboxkit config show
sboxkit config set --key enable_tun --value false
sboxkit config set --key lan_panel --value true
sboxkit config set --key direct_domain_suffixes --value example.com,example.org
```

After enabling the WebUI, rebuild and sync the active config:

```bash
sboxkit sub rebuild --name main
sudo sboxkit service sync
```

Open:

```text
http://127.0.0.1:9090/ui/
```

## Shell Proxy Environment

```bash
sboxkit proxy-env write
sboxkit proxy-env remove
```

The managed block exports:

```text
http_proxy=http://127.0.0.1:7890
https_proxy=http://127.0.0.1:7890
all_proxy=socks5://127.0.0.1:7890
```

## Node Operations

```bash
sboxkit node list
sboxkit node list --api http://127.0.0.1:9090
sboxkit node list --api http://127.0.0.1:9090 --secret TOKEN
sboxkit node switch --group Proxy --name NODE
```

Node switching uses the running sing-box Clash API and does not restart the
service.

## Timers And Resilience

```bash
sudo sboxkit timer install --binary /usr/bin/sboxkit
sudo sboxkit timer remove

sudo sboxkit resilience install
sudo sboxkit resilience remove
```

## Diagnostics

```bash
sboxkit nettest
sboxkit ui serve --addr 127.0.0.1:8790
```

`ui serve` is only a local asset preview helper. The normal WebUI path is served
by sing-box after `lan_panel` is enabled and the service runtime is synced.

## Uninstall

```bash
sudo sboxkit uninstall
sudo sboxkit uninstall --purge-state
```

`sboxkit uninstall` removes sboxkit-managed service files, timers, resilience
hooks, `/etc/sboxkit`, and optionally user state. It does not remove the Debian
package that installed `/usr/bin/sboxkit`.

Remove the installed package with:

```bash
sudo apt remove sboxkit
```

Remove package conffiles as well with:

```bash
sudo apt purge sboxkit
```
