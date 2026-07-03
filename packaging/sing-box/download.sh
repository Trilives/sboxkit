#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: download.sh --goarch ARCH [--goarm 7] --out-dir DIR [--version latest]

Downloads the upstream sing-box Linux tar.gz release for the target architecture,
extracts the independent sing-box binary, and writes VERSION and SOURCE_URL files.
USAGE
}

goarch=""
goarm=""
out_dir=""
version="latest"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --goarch)
      goarch="${2:-}"; shift 2 ;;
    --goarm)
      goarm="${2:-}"; shift 2 ;;
    --out-dir)
      out_dir="${2:-}"; shift 2 ;;
    --version)
      version="${2:-}"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2 ;;
  esac
done

if [ -z "$goarch" ] || [ -z "$out_dir" ]; then
  usage >&2
  exit 2
fi

case "$goarch:${goarm}" in
  amd64:) asset_arch="amd64" ;;
  arm64:) asset_arch="arm64" ;;
  arm:7) asset_arch="armv7" ;;
  *)
    echo "unsupported sing-box target: GOARCH=${goarch} GOARM=${goarm}" >&2
    exit 2 ;;
esac

mkdir -p "$out_dir"
meta="$out_dir/release.json"

if command -v gh >/dev/null 2>&1; then
  if [ "$version" = "latest" ]; then
    gh api repos/SagerNet/sing-box/releases/latest > "$meta"
  else
    gh api "repos/SagerNet/sing-box/releases/tags/${version}" > "$meta"
  fi
else
  if [ "$version" = "latest" ]; then
    curl -fsSL -H 'Accept: application/vnd.github+json' \
      https://api.github.com/repos/SagerNet/sing-box/releases/latest > "$meta"
  else
    curl -fsSL -H 'Accept: application/vnd.github+json' \
      "https://api.github.com/repos/SagerNet/sing-box/releases/tags/${version}" > "$meta"
  fi
fi

tag="$(
  python3 - "$meta" <<'PY'
import json, sys
print(json.load(open(sys.argv[1], encoding="utf-8"))["tag_name"])
PY
)"
plain_version="${tag#v}"
asset_name="sing-box-${plain_version}-linux-${asset_arch}.tar.gz"
asset_url="$(
  python3 - "$meta" "$asset_name" <<'PY'
import json, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
name = sys.argv[2]
for asset in data.get("assets", []):
    if asset.get("name") == name:
        print(asset["browser_download_url"])
        break
else:
    raise SystemExit(f"asset not found: {name}")
PY
)"

archive="$out_dir/$asset_name"
if command -v gh >/dev/null 2>&1; then
  if ! gh release download "$tag" --repo SagerNet/sing-box --pattern "$asset_name" --dir "$out_dir" --clobber; then
    curl -fL -o "$archive" "$asset_url"
  fi
else
  curl -fL -o "$archive" "$asset_url"
fi

extract_dir="$out_dir/extract"
rm -rf "$extract_dir"
mkdir -p "$extract_dir"
tar -C "$extract_dir" -xzf "$archive"
found="$(find "$extract_dir" -type f -name sing-box -perm /111 | head -n 1)"
if [ -z "$found" ]; then
  echo "sing-box binary not found in $asset_name" >&2
  exit 1
fi
install -m 0755 "$found" "$out_dir/sing-box"
printf '%s\n' "$tag" > "$out_dir/VERSION"
printf '%s\n' "https://github.com/SagerNet/sing-box/tree/${tag}" > "$out_dir/SOURCE_URL"
printf '%s\n' "$asset_url" > "$out_dir/BINARY_URL"
