#!/usr/bin/env bash
# One-click installer for sboxkit.
#
#   curl -fsSL https://github.com/Trilives/sboxkit/releases/latest/download/install.sh | bash
#   curl -fsSL https://github.com/Trilives/sboxkit/releases/latest/download/install.sh | bash -s -- --user
#
# System mode (default) installs the release .deb with dpkg; sudo is asked for
# that step only. User mode unpacks the user-mode bundle into
# ~/.local/share/sboxkit, links ~/.local/bin/sboxkit and never uses sudo.
# Nothing is configured here: run `sboxkit` afterwards.
#
# Every download is checked against the SHA-256 digest api.github.com reports
# for the release asset; --mirror only carries the bytes and is never asked for
# the digest. Fetching this script itself through a mirror trusts that mirror.
set -euo pipefail

REPO="Trilives/sboxkit"
API="https://api.github.com/repos/${REPO}"
DOWNLOAD="https://github.com/${REPO}/releases/download"
API_HINT="a 403 means the hourly rate limit is used up: retry later or set GITHUB_TOKEN; set HTTPS_PROXY if GitHub is blocked"

usage() {
  cat <<'EOF'
Usage: install.sh [--user] [--version X.Y.Z] [--mirror URL] [--dir DIR]

  --user           user mode: no sudo, systemd user service, data in DIR
  --version X.Y.Z  install this release instead of the latest one
  --mirror URL     download release assets through an https GitHub mirror
                   (e.g. https://ghfast.top); digests still come from GitHub
  --dir DIR        user-mode install directory (default ~/.local/share/sboxkit)

SBOXKIT_MIRROR=URL in the environment works like --mirror; GITHUB_TOKEN, when
set, is sent to api.github.com only (the anonymous limit is 60 requests/hour
per IP, easily used up behind a shared proxy).
EOF
}

info() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

# release_arch maps `uname -m` to the arch suffix of the release assets.
release_arch() {
  case "$1" in
    x86_64 | amd64) echo amd64 ;;
    aarch64 | arm64) echo arm64 ;;
    armv7* | armhf) echo armv7 ;;
    *) return 1 ;;
  esac
}

api_get() {
  local auth=()
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    auth=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
  fi
  curl -fsSL --proto '=https' --proto-redir '=https' --retry 3 --retry-delay 2 \
    -H "Accept: application/vnd.github+json" ${auth[@]+"${auth[@]}"} "$1"
}

# download fetches $1 to $2, resuming after an interrupted transfer (curl's own
# --retry does not cover "transferred a partial file").
download() {
  local try rc
  rm -f "$2"
  for try in 1 2 3 4 5; do
    curl -fL --proto '=https' --proto-redir '=https' --retry 3 --retry-delay 2 -C - -o "$2" "$1" && return 0
    rc=$?
    [[ ${rc} -ne 22 ]] || break # HTTP error status: retrying will not help
    warn "download interrupted (${try}/5), resuming"
    sleep 2
  done
  rm -f "$2"
  return 1
}

mirrored() {
  if [[ -n "${MIRROR}" ]]; then
    printf '%s/%s' "${MIRROR%/}" "$1"
  else
    printf '%s' "$1"
  fi
}

# json_tokens prints the `"key": value` pairs named by the regex $1 from JSON on
# stdin, one per line. Readers consume all of it, so `pipefail` never sees a
# SIGPIPE, and no match is not an error.
json_tokens() {
  tr -d '\n' | { grep -oE "\"($1)\" *: *(\"[^\"]*\"|null)" || true; }
}

# tag_name prints the release tag from release JSON on stdin.
tag_name() {
  json_tokens tag_name | awk -F'"' 'NR == 1 { print $4 }'
}

# asset_digest prints the `digest` of asset $1 from release JSON on stdin:
# "sha256:<hex>", "null" when the release predates asset digests, nothing when
# the asset does not exist. Within an asset object `digest` follows `name`, and
# quotes inside JSON strings are escaped, so release notes cannot fake a match.
asset_digest() {
  json_tokens 'name|digest' | awk -v want="$1" '
    done { next }
    /^"name"/ { n = $0; sub(/^"name" *: *"/, "", n); sub(/"$/, "", n); hit = (n == want); if (hit) found = 1; next }
    hit { d = $0; sub(/^"digest" *: *"?/, "", d); sub(/"$/, "", d); print d; done = 1 }
    END { if (found && !done) print "null" }'
}

# expected_sha256 resolves the trusted SHA-256 of asset $1. Without an asset
# digest it falls back to the release's checksums.txt, read straight from
# GitHub over TLS; through a mirror that would trust the mirror, so it refuses.
expected_sha256() {
  local asset="$1" digest
  digest="$(asset_digest "${asset}" <<<"${RELEASE_JSON}")"
  [[ -n "${digest}" ]] || die "release v${VERSION} has no asset ${asset}"
  if [[ "${digest}" != null ]]; then
    [[ "${digest}" =~ ^sha256:[0-9a-fA-F]{64}$ ]] || die "unexpected digest for ${asset}: ${digest}"
    printf '%s' "${digest#sha256:}"
    return
  fi
  [[ -z "${MIRROR}" ]] || die "GitHub reports no digest for ${asset}; refusing to install it through a mirror"
  download "${DOWNLOAD}/v${VERSION}/checksums.txt" "${TMP}/checksums.txt" || die "cannot download checksums.txt"
  awk -v n="${asset}" '!found && ($2 == n || $2 == "*" n) { print $1; found = 1 }' "${TMP}/checksums.txt"
}

# fetch_verified downloads release asset $1 to $2, checks its SHA-256 and sets
# WANT_SHA256 to the expected digest.
fetch_verified() {
  local asset="$1" out="$2" got
  WANT_SHA256="$(expected_sha256 "${asset}")"
  [[ "${WANT_SHA256}" =~ ^[0-9a-fA-F]{64}$ ]] || die "no usable checksum published for ${asset}"
  info "Downloading ${asset}"
  download "$(mirrored "${DOWNLOAD}/v${VERSION}/${asset}")" "${out}" || die "cannot download ${asset}"
  got="$(sha256sum "${out}" | cut -d' ' -f1)"
  if [[ "${got,,}" != "${WANT_SHA256,,}" ]]; then
    rm -f "${out}"
    die "SHA-256 mismatch for ${asset} (expected ${WANT_SHA256}, got ${got}); nothing was installed"
  fi
  info "SHA-256 verified"
}

as_root() {
  if [[ ${EUID} -eq 0 ]]; then
    "$@"
    return
  fi
  command -v sudo >/dev/null || die "installing the .deb needs root; run as root or install sudo"
  sudo "$@"
}

# install_deb copies the downloaded .deb into a root-only directory, checks the
# digest again there and installs that copy: the download directory belongs to
# the invoking user, so the file could be swapped while sudo asks for a password.
install_deb() {
  as_root sh -c '
    set -eu
    dir="$(mktemp -d)"
    trap '\''rm -rf "$dir"'\'' EXIT
    cp "$1" "$dir/sboxkit.deb"
    printf "%s  %s\n" "$2" "$dir/sboxkit.deb" | sha256sum -c --status - || {
      echo "error: the package changed after it was verified; nothing was installed" >&2
      exit 1
    }
    dpkg -i "$dir/sboxkit.deb"
  ' sh "$1" "$2"
}

# deb_version prints how dpkg records release version $1: nfpm writes a
# pre-release suffix the Debian way (0.4.1-beta.1 becomes 0.4.1~beta.1) so that
# it sorts below the final release.
deb_version() {
  printf '%s\n' "${1/-/\~}"
}

install_system() {
  command -v dpkg >/dev/null || die "system mode needs dpkg (Debian/Ubuntu); use --user on other distributions"
  local installed asset
  installed="$(dpkg-query -W -f='${db:Status-Status} ${Version}' sboxkit 2>/dev/null || true)"
  if [[ "${installed}" == "installed $(deb_version "${VERSION}")" ]]; then
    info "sboxkit ${VERSION} is already installed; run: sboxkit"
    return
  fi
  asset="sboxkit_${VERSION}_linux_${ARCH}.deb"
  fetch_verified "${asset}" "${TMP}/${asset}"
  info "Installing ${asset} with dpkg"
  install_deb "${TMP}/${asset}" "${WANT_SHA256,,}"
  info "Done. Run: sboxkit"
}

install_user() {
  [[ ${EUID} -ne 0 ]] || die "user mode runs as your own user; run the installer without sudo"
  local dir="${DIR:-${XDG_DATA_HOME:-${HOME}/.local/share}/sboxkit}"
  local bin_dir="${HOME}/.local/bin" asset
  if [[ -e "${dir}/sboxkit" ]]; then
    die "sboxkit is already installed in ${dir}; update it from Tools > Update inside sboxkit"
  fi
  asset="sboxkit-user_${VERSION}_linux_${ARCH}.tar.gz"
  fetch_verified "${asset}" "${TMP}/${asset}"
  mkdir -p "${dir}" "${bin_dir}"
  tar -xzf "${TMP}/${asset}" -C "${dir}" --strip-components=1
  if [[ -e "${bin_dir}/sboxkit" && ! -L "${bin_dir}/sboxkit" ]]; then
    warn "${bin_dir}/sboxkit exists and is not a link; leaving it alone. Run: ${dir}/sboxkit"
  else
    ln -sfn "${dir}/sboxkit" "${bin_dir}/sboxkit"
  fi
  case ":${PATH}:" in
    *":${bin_dir}:"*) info "Done. Run: sboxkit" ;;
    *) info "Done. Run: ${dir}/sboxkit (or add ${bin_dir} to PATH)" ;;
  esac
}

main() {
  MODE=system VERSION="" MIRROR="${SBOXKIT_MIRROR:-}" DIR=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --user) MODE=user ;;
      --system) MODE=system ;;
      --version) VERSION="${2:?--version needs a value}" && shift ;;
      --version=*) VERSION="${1#*=}" ;;
      --mirror) MIRROR="${2:?--mirror needs a URL}" && shift ;;
      --mirror=*) MIRROR="${1#*=}" ;;
      --dir) DIR="${2:?--dir needs a path}" && shift ;;
      --dir=*) DIR="${1#*=}" ;;
      -h | --help)
        usage
        return
        ;;
      *) die "unknown option: $1 (see --help)" ;;
    esac
    shift
  done
  [[ "$(uname -s)" == Linux ]] || die "sboxkit only runs on Linux"
  for tool in curl sha256sum tar; do
    command -v "${tool}" >/dev/null || die "${tool} is required"
  done
  ARCH="$(release_arch "$(uname -m)")" || die "unsupported architecture: $(uname -m)"
  [[ -z "${MIRROR}" || "${MIRROR}" =~ ^https:// ]] || die "--mirror must be an https URL"
  [[ -z "${VERSION}" || "${VERSION}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+([.+~-][0-9A-Za-z.+~-]*)?$ ]] ||
    die "invalid --version: ${VERSION} (expected X.Y.Z)"

  TMP="$(mktemp -d)"
  trap 'rm -rf "${TMP}"' EXIT

  VERSION="${VERSION#v}"
  if [[ -n "${VERSION}" ]]; then
    RELEASE_JSON="$(api_get "${API}/releases/tags/v${VERSION}")" || die "cannot read release v${VERSION} from api.github.com (${API_HINT})"
  else
    RELEASE_JSON="$(api_get "${API}/releases/latest")" || die "cannot read the latest release from api.github.com (${API_HINT})"
    VERSION="$(tag_name <<<"${RELEASE_JSON}")"
    VERSION="${VERSION#v}"
    [[ -n "${VERSION}" ]] || die "cannot determine the latest release"
  fi
  info "sboxkit ${VERSION} (${MODE} mode, linux/${ARCH})"

  if [[ "${MODE}" == user ]]; then
    install_user
  else
    install_system
  fi
}

# Everything runs from main so a truncated `curl | bash` download does nothing;
# the tests source this file with SBOXKIT_INSTALL_LIB=1 to reach the helpers.
if [[ "${SBOXKIT_INSTALL_LIB:-}" != 1 ]]; then
  main "$@"
fi
