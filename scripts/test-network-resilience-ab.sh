#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
HARNESS="${ROOT}/scripts/network-resilience-ab.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local file="$1"
  local expected="$2"
  grep -Fq -- "${expected}" "${file}" || fail "${file} does not contain: ${expected}"
}

assert_contains "${HARNESS}" "umask 077"
assert_contains "${HARNESS}" "#!/usr/bin/bash"
assert_contains "${HARNESS}" "export PATH=/usr/sbin:/usr/bin:/sbin:/bin"
assert_contains "${HARNESS}" "stage_trusted_cores"
if grep -Fq -- "or tcp port" "${HARNESS}"; then
  fail "harness captures mixed proxy traffic"
fi

test_root="$(mktemp -d)"
trap 'rm -rf -- "${test_root}"' EXIT

configs="${test_root}/configs"
mkdir -p "${configs}"
for tun in on off; do
  for fakeip in on off; do
    tun_json=''
    fakeip_json=''
    if [[ "${tun}" == on ]]; then
      tun_json='{"type":"tun","tag":"tun-in"},'
    fi
    if [[ "${fakeip}" == on ]]; then
      fakeip_json=',{"type":"fakeip","tag":"dns-fakeip"}'
    fi
    printf '{"inbounds":[%s{"type":"mixed","listen_port":7890}],"dns":{"servers":[{"type":"tcp","server":"198.18.0.2"}%s]}}\n' \
      "${tun_json}" "${fakeip_json}" > "${configs}/tun-${tun}-fakeip-${fakeip}.json"
  done
done

make_fake_core() {
  local path="$1"
  local version="$2"
  sed "s/@VERSION@/${version}/g" > "${path}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  version) printf 'sing-box version @VERSION@\n' ;;
  check) exit 0 ;;
  run) trap 'exit 0' TERM INT; while :; do sleep 1; done ;;
  *) exit 2 ;;
esac
EOF
  chmod 0755 "${path}"
}

core_11318="${test_root}/sing-box-1.13.18"
core_11319="${test_root}/sing-box-1.13.19"
make_fake_core "${core_11318}" "1.13.18"
make_fake_core "${core_11319}" "1.13.19"

common_env=(
  "SING_BOX_11318_BIN=${core_11318}"
  "SING_BOX_11319_BIN=${core_11319}"
  "AB_CONFIG_DIR=${configs}"
  "AB_RUN_ID=test-run"
)

default_out="${test_root}/default.out"
if "${HARNESS}" >"${default_out}" 2>&1; then
  fail "invocation without a mode must be refused"
fi
assert_contains "${default_out}" "refusing to touch networking"

missing_out="${test_root}/missing.out"
if env -u SING_BOX_11318_BIN -u SING_BOX_11319_BIN \
  AB_CONFIG_DIR="${configs}" AB_RUN_ID=test-run \
  "${HARNESS}" dry-run >"${missing_out}" 2>&1; then
  fail "dry-run without explicit binaries must fail"
fi
assert_contains "${missing_out}" "SING_BOX_11318_BIN"

mock_bin="${test_root}/mock-bin"
mkdir -p "${mock_bin}"
ip_marker="${test_root}/ip-was-called"
cat > "${mock_bin}/ip" <<EOF
#!/usr/bin/env bash
touch "${ip_marker}"
exit 99
EOF
chmod 0755 "${mock_bin}/ip"

dry_out="${test_root}/dry.out"
env "${common_env[@]}" PATH="${mock_bin}:${PATH}" \
  "${HARNESS}" dry-run >"${dry_out}"
assert_contains "${dry_out}" "MODE=dry-run"
assert_contains "${dry_out}" "matrix_cases=32"
assert_contains "${dry_out}" "version=1.13.18 fault=tcp-rst tun=on fakeip=on"
assert_contains "${dry_out}" "version=1.13.19 fault=uplink-down tun=off fakeip=off"
[[ ! -e "${ip_marker}" ]] || fail "dry-run invoked ip and was not side-effect free"

verify_out="${test_root}/verify.out"
env "${common_env[@]}" "${HARNESS}" verify >"${verify_out}"
assert_contains "${verify_out}" "verified sing-box 1.13.18"
assert_contains "${verify_out}" "verified sing-box 1.13.19"
assert_contains "${verify_out}" "verified configuration matrix: 4 files"

unsafe_out="${test_root}/unsafe.out"
if env "${common_env[@]}" AB_RUN_ID='../host' \
  "${HARNESS}" dry-run >"${unsafe_out}" 2>&1; then
  fail "unsafe run id must be rejected"
fi
assert_contains "${unsafe_out}" "unsafe AB_RUN_ID"

wrong_core="${test_root}/sing-box-wrong"
make_fake_core "${wrong_core}" "1.13.17"
wrong_out="${test_root}/wrong.out"
if env "${common_env[@]}" SING_BOX_11318_BIN="${wrong_core}" \
  "${HARNESS}" verify >"${wrong_out}" 2>&1; then
  fail "mismatched core version must be rejected"
fi
assert_contains "${wrong_out}" "expected 1.13.18"

if [[ "${EUID}" -ne 0 ]]; then
  root_out="${test_root}/root.out"
  if env "${common_env[@]}" AB_CONFIRM_ISOLATED=yes \
    AB_OUTPUT_DIR="${test_root}/must-not-exist" PATH="${mock_bin}:${PATH}" \
    "${HARNESS}" run >"${root_out}" 2>&1; then
    fail "active mode as non-root must be rejected"
  fi
  assert_contains "${root_out}" "requires root"
  [[ ! -e "${ip_marker}" ]] || fail "non-root rejection invoked ip"
fi

echo "network resilience A/B harness self-test: PASS"
