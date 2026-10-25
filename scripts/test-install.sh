#!/usr/bin/env bash
# Offline tests for scripts/install.sh: release parsing, digest selection and
# checksum enforcement. Downloads are stubbed; nothing is installed.
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=install.sh
SBOXKIT_INSTALL_LIB=1 source "${ROOT}/scripts/install.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  [[ "$1" == "$2" ]] || fail "$3: got '$1', want '$2'"
}

TMP="$(mktemp -d)"
trap 'rm -rf -- "${TMP}"' EXIT
MIRROR="" VERSION=9.9.9

payload="${TMP}/payload"
printf 'genuine' >"${payload}"
good="$(sha256sum "${payload}" | cut -d' ' -f1)"
bad="$(printf 'other' | sha256sum | cut -d' ' -f1)"

# Pretty-printed like api.github.com, with release notes that quote a field.
RELEASE_JSON="$(cat <<EOF
{
  "tag_name": "v9.9.9",
  "name": "v9.9.9",
  "body": "notes \"name\": \"sboxkit_9.9.9_linux_amd64.deb\", \"digest\": \"sha256:${bad}\"",
  "assets": [
    {
      "name": "sboxkit_9.9.9_linux_amd64.deb",
      "uploader": {"login": "github-actions[bot]"},
      "digest": "sha256:${good}",
      "browser_download_url": "https://github.com/Trilives/sboxkit/releases/download/v9.9.9/sboxkit_9.9.9_linux_amd64.deb"
    },
    {"name": "sboxkit-user_9.9.9_linux_amd64.tar.gz", "digest": null},
    {"name": "sboxkit_9.9.9_linux_arm64.deb", "digest": "sha256:${bad}"}
  ]
}
EOF
)"

for pair in x86_64:amd64 aarch64:arm64 armv7l:armv7 armhf:armv7; do
  assert_eq "$(release_arch "${pair%%:*}")" "${pair#*:}" "release_arch ${pair%%:*}"
done
if release_arch riscv64 >/dev/null; then
  fail "riscv64 has no release asset and must be rejected"
fi

assert_eq "$(deb_version 0.4.1-beta.1)" "0.4.1~beta.1" "dpkg version of a preview"
assert_eq "$(deb_version 0.4.0)" "0.4.0" "dpkg version of a stable release"

assert_eq "$(tag_name <<<"${RELEASE_JSON}")" "v9.9.9" "tag_name"
assert_eq "$(tag_name <<<'{"message": "Not Found"}')" "" "tag_name without a release"
assert_eq "$(tr -d '\n' <<<"${RELEASE_JSON}" | asset_digest sboxkit_9.9.9_linux_amd64.deb)" "sha256:${good}" "digest from the asset, not the notes"
assert_eq "$(asset_digest sboxkit-user_9.9.9_linux_amd64.tar.gz <<<"${RELEASE_JSON}")" "null" "asset without digest"
assert_eq "$(asset_digest sboxkit_9.9.9_linux_armv7.deb <<<"${RELEASE_JSON}")" "" "missing asset"

assert_eq "$(mirrored https://github.com/a)" "https://github.com/a" "no mirror"
assert_eq "$(MIRROR=https://m.example/ mirrored https://github.com/a)" "https://m.example/https://github.com/a" "mirror prefix"

# Downloads: release assets come from ${payload}; checksums.txt lists the user bundle.
download() {
  case "$1" in
    */checksums.txt) printf '%s  sboxkit-user_9.9.9_linux_amd64.tar.gz\n' "${good}" >"$2" ;;
    *) cp "${payload}" "$2" ;;
  esac
}

fetch_verified sboxkit_9.9.9_linux_amd64.deb "${TMP}/a.deb" >/dev/null
fetch_verified sboxkit-user_9.9.9_linux_amd64.tar.gz "${TMP}/u.tar.gz" >/dev/null

if (fetch_verified sboxkit_9.9.9_linux_arm64.deb "${TMP}/b.deb") >/dev/null 2>&1; then
  fail "content that does not match the digest was accepted"
fi
[[ ! -e "${TMP}/b.deb" ]] || fail "a rejected download was left behind"
if (MIRROR=https://m.example fetch_verified sboxkit-user_9.9.9_linux_amd64.tar.gz "${TMP}/m.tar.gz") >/dev/null 2>&1; then
  fail "an asset without a GitHub digest was accepted through a mirror"
fi
if (fetch_verified sboxkit_9.9.9_linux_armv7.deb "${TMP}/c.deb") >/dev/null 2>&1; then
  fail "a missing asset was accepted"
fi

# install_deb re-checks a root-owned copy and hands only that copy to dpkg.
mkdir -p "${TMP}/bin"
printf '#!/bin/sh\nprintf "%%s\\n" "$*" >>"%s/dpkg.log"\n' "${TMP}" >"${TMP}/bin/dpkg"
chmod +x "${TMP}/bin/dpkg"
as_root() { PATH="${TMP}/bin:${PATH}" "$@"; }
install_deb "${payload}" "${good}" >/dev/null
grep -q '^-i .*/sboxkit.deb$' "${TMP}/dpkg.log" || fail "dpkg did not get the verified copy"
if grep -qF "${payload}" "${TMP}/dpkg.log"; then
  fail "dpkg was given the user-writable download"
fi
: >"${TMP}/dpkg.log"
if (install_deb "${payload}" "${bad}") >/dev/null 2>&1; then
  fail "a package that changed after verification was installed"
fi
[[ ! -s "${TMP}/dpkg.log" ]] || fail "dpkg ran on a package that failed the second check"

echo "install.sh tests passed"
