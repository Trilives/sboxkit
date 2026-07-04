#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: build-portable.sh --binary PATH --sing-box PATH --version VERSION --arch ARCH --out-dir DIR [--sing-box-version VERSION] [--sing-box-source URL]
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

root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT

bundle="$root/sboxkit_${version}_${arch}"
mkdir -p "$bundle"
install -m 0755 "$binary" "$bundle/sboxkit"
install -m 0755 "$sing_box" "$bundle/sing-box"
mkdir -p "$bundle/base-rules"
mkdir -p "$bundle/ui"
mkdir -p "$bundle/docs"
mkdir -p "$bundle/scripts"
install -m 0644 packaging/base-rules/minimal.json "$bundle/base-rules/minimal.json"
install -m 0755 packaging/migrations/sboxkit-migrate-legacy.sh "$bundle/scripts/sboxkit-migrate-legacy.sh"
cp -a internal/uiassets/assets/. "$bundle/ui/"
install -m 0644 README.md "$bundle/README.md"
install -m 0644 LICENSE "$bundle/LICENSE"
install -m 0644 docs/COMMANDS.md "$bundle/docs/COMMANDS.md"
install -m 0644 docs/THIRD_PARTY_ASSETS.md "$bundle/THIRD_PARTY_ASSETS.md"
cat > "$bundle/SING_BOX_SOURCE.txt" <<SOURCE
sing-box binary included in this portable bundle
Version: ${sing_box_version}
Corresponding source: ${sing_box_source}
License: GPL-3.0-or-later (per upstream project)

sing-box is distributed as an independent executable next to sboxkit.
It is not linked into the sboxkit binary.
SOURCE

cat > "$bundle/install.sh" <<'INSTALL'
#!/usr/bin/env sh
set -e
prefix="${PREFIX:-/usr}"
install_bin="${prefix}/bin"
install_lib="${prefix}/lib/sboxkit"
install_share="${prefix}/share/sboxkit/base-rules"
install_ui="${prefix}/share/sboxkit/ui"

if [ "$(id -u)" -ne 0 ]; then
  echo "install.sh must run as root. Try: sudo ./install.sh" >&2
  exit 1
fi

install -d -m 0755 "$install_bin" "$install_lib" "$install_share" "$install_ui"
install -m 0755 ./sboxkit "$install_bin/sboxkit"
install -m 0755 ./sing-box "$install_lib/sing-box"
install -m 0644 ./base-rules/minimal.json "$install_share/minimal.json"
cp -a ./ui/. "$install_ui/"

echo "Installed sboxkit to $install_bin/sboxkit"
echo "Installed sing-box core to $install_lib/sing-box"
echo "Installed sboxkit WebUI to $install_ui"
echo "Run: sboxkit init"
INSTALL
chmod 0755 "$bundle/install.sh"

mkdir -p "$out_dir"
tar -C "$root" -czf "$out_dir/sboxkit_${version}_${arch}_portable.tar.gz" "$(basename "$bundle")"
