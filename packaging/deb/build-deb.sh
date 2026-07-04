#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: build-deb.sh --binary PATH --sing-box PATH --version VERSION --arch DEB_ARCH --out-dir DIR [--sing-box-version VERSION] [--sing-box-source URL]

Builds a Debian package containing sboxkit and the independent upstream
sing-box binary as separate files. sing-box is not linked into sboxkit.
USAGE
}

binary=""
sing_box=""
version=""
arch=""
out_dir="dist"
sing_box_version="unknown"
sing_box_source="https://github.com/SagerNet/sing-box"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --binary)
      binary="${2:-}"; shift 2 ;;
    --sing-box)
      sing_box="${2:-}"; shift 2 ;;
    --version)
      version="${2:-}"; shift 2 ;;
    --arch)
      arch="${2:-}"; shift 2 ;;
    --out-dir)
      out_dir="${2:-}"; shift 2 ;;
    --sing-box-version)
      sing_box_version="${2:-}"; shift 2 ;;
    --sing-box-source)
      sing_box_source="${2:-}"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2 ;;
  esac
done

if [ -z "$binary" ] || [ -z "$sing_box" ] || [ -z "$version" ] || [ -z "$arch" ]; then
  usage >&2
  exit 2
fi
if [ ! -x "$binary" ]; then
  echo "binary is not executable: $binary" >&2
  exit 1
fi
if [ ! -x "$sing_box" ]; then
  echo "sing-box binary is not executable: $sing_box" >&2
  exit 1
fi

case "$arch" in
  amd64|arm64|armhf) ;;
  *)
    echo "unsupported Debian architecture: $arch" >&2
    exit 2 ;;
esac

root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT

pkg="$root/sboxkit_${version}_${arch}"
mkdir -p \
  "$pkg/DEBIAN" \
  "$pkg/lib/systemd/system" \
  "$pkg/usr/bin" \
  "$pkg/usr/lib/sboxkit" \
  "$pkg/usr/share/sboxkit/base-rules" \
  "$pkg/usr/share/sboxkit/scripts" \
  "$pkg/usr/share/sboxkit/ui" \
  "$pkg/usr/share/doc/sboxkit/docs" \
  "$pkg/usr/share/doc/sboxkit" \
  "$pkg/usr/share/licenses/sboxkit"

install -m 0755 "$binary" "$pkg/usr/bin/sboxkit"
install -m 0755 "$sing_box" "$pkg/usr/lib/sboxkit/sing-box"
install -m 0755 packaging/migrations/sboxkit-migrate-legacy.sh "$pkg/usr/share/sboxkit/scripts/sboxkit-migrate-legacy.sh"
install -m 0644 packaging/base-rules/minimal.json "$pkg/usr/share/sboxkit/base-rules/minimal.json"
cp -a internal/uiassets/assets/. "$pkg/usr/share/sboxkit/ui/"
install -m 0644 README.md "$pkg/usr/share/doc/sboxkit/README.md"
install -m 0644 ARCHITECTURE.md "$pkg/usr/share/doc/sboxkit/ARCHITECTURE.md"
install -m 0644 docs/COMMANDS.md "$pkg/usr/share/doc/sboxkit/docs/COMMANDS.md"
install -m 0644 docs/THIRD_PARTY_ASSETS.md "$pkg/usr/share/doc/sboxkit/THIRD_PARTY_ASSETS.md"
install -m 0644 LICENSE "$pkg/usr/share/licenses/sboxkit/LICENSE"
install -m 0644 packaging/deb/copyright "$pkg/usr/share/doc/sboxkit/copyright"
cat > "$pkg/usr/share/doc/sboxkit/SING_BOX_SOURCE.txt" <<SOURCE
sing-box binary included in this package
Version: ${sing_box_version}
Corresponding source: ${sing_box_source}
License: GPL-3.0-or-later (per upstream project)

sing-box is distributed as an independent executable at:
/usr/lib/sboxkit/sing-box

It is not linked into /usr/bin/sboxkit.
SOURCE

cat > "$pkg/lib/systemd/system/sboxkit.service" <<'UNIT'
[Unit]
Description=sboxkit managed sing-box service
Documentation=https://sing-box.sagernet.org/
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/var/lib/sboxkit/current
ExecStart=/usr/lib/sboxkit/sing-box run -c /var/lib/sboxkit/current/config.json
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW

[Install]
WantedBy=multi-user.target
UNIT

installed_size="$(du -ks "$pkg" | awk '{print $1}')"

cat > "$pkg/DEBIAN/control" <<CONTROL
Package: sboxkit
Version: ${version}
Section: net
Priority: optional
Architecture: ${arch}
Maintainer: Trilives <noreply@github.com>
Installed-Size: ${installed_size}
Depends: ca-certificates, curl, iproute2
Recommends: systemd
Homepage: https://github.com/Trilives/sboxkit
Description: Linux CLI deployment manager bundled with sing-box core
 sboxkit manages sing-box subscriptions, generated configuration, runtime
 asset downloads, systemd service setup, update timers, and network resilience.
 This package contains two independent executables: the sboxkit manager and
 the upstream sing-box core. The sing-box core is not linked into sboxkit.
CONTROL

cat > "$pkg/DEBIAN/postinst" <<'POSTINST'
#!/usr/bin/env sh
set -e
echo "sboxkit installed. Run: sboxkit init"
echo "Breaking layout change: if upgrading from an older release, run the migration helper first if you want to preserve old state:"
echo "  sudo /usr/share/sboxkit/scripts/sboxkit-migrate-legacy.sh --yes"
exit 0
POSTINST
chmod 0755 "$pkg/DEBIAN/postinst"

mkdir -p "$out_dir"
dpkg-deb --build --root-owner-group "$pkg" "$out_dir/sboxkit_${version}_${arch}.deb"
