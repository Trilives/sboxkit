#!/usr/bin/bash
# Privileged sing-box 1.13.18/1.13.19 resilience experiment harness.
#
# Safety boundary: this script never changes host routes, sysctls, or firewall
# rules. Active modes create two throw-away network namespaces joined by one
# veth pair; fault rules exist only inside the upstream namespace.
set -euo pipefail
umask 077
export PATH=/usr/sbin:/usr/bin:/sbin:/bin

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly DNS_FIXTURE="${SCRIPT_DIR}/network-resilience-no-response.py"
readonly MODE="${1:-}"
readonly RUN_ID="${AB_RUN_ID:-}"
readonly CLIENT_NS="sboxkit-ab-${RUN_ID}-client"
readonly UPSTREAM_NS="sboxkit-ab-${RUN_ID}-upstream"
readonly CLIENT_IF="ab-client0"
readonly UPSTREAM_IF="ab-upstream0"
readonly CLIENT_IP="${AB_CLIENT_IP:-198.18.0.1}"
readonly UPSTREAM_IP="${AB_UPSTREAM_IP:-198.18.0.2}"
readonly CONFIG_DIR="${AB_CONFIG_DIR:-}"
readonly OUTPUT_DIR="${AB_OUTPUT_DIR:-/tmp/sboxkit-network-resilience-ab-${RUN_ID}}"
readonly OWNERSHIP_FILE="/run/sboxkit-ab-${RUN_ID}.owner"
readonly MIXED_PORT="${AB_MIXED_PORT:-7890}"
readonly PROBE_URL="${AB_PROBE_URL:-http://connectivitycheck.gstatic.com/generate_204}"
readonly SETTLE_SECONDS="${AB_SETTLE_SECONDS:-2}"
readonly PROBE_TIMEOUT="${AB_PROBE_TIMEOUT:-8}"
readonly EXPECTED_11318_SHA256="${SING_BOX_11318_SHA256:-}"
readonly EXPECTED_11319_SHA256="${SING_BOX_11319_SHA256:-}"

readonly -a VERSIONS=("1.13.18" "1.13.19")
readonly -a FAULTS=("tcp-rst" "tcp-blackhole" "dns-no-response" "uplink-down")
readonly -a TUN_STATES=("on" "off")
readonly -a FAKEIP_STATES=("on" "off")

declare -A CORE_BY_VERSION=(
  ["1.13.18"]="${SING_BOX_11318_BIN:-}"
  ["1.13.19"]="${SING_BOX_11319_BIN:-}"
)

topology_active=0
veth_active=0
core_pid=""
fixture_pid=""
tcpdump_pid=""
host_client_if=""
host_upstream_if=""
trusted_core_dir=""

usage() {
  cat <<'EOF'
Usage: network-resilience-ab.sh MODE

Modes:
  dry-run  Validate explicit inputs and print the 32-case matrix. No root needed.
  verify   Verify both core versions and all four config files. No root needed.
  prepare  As root, create and inspect one isolated topology, then remove it.
  run      As root, execute and archive the complete isolated fault matrix.
  cleanup  As root, remove only this run-id's two prefixed namespaces.

Required environment:
  SING_BOX_11318_BIN   Native sing-box 1.13.18 executable
  SING_BOX_11319_BIN   Native sing-box 1.13.19 executable
  AB_CONFIG_DIR        Directory containing these exact files:
                         tun-on-fakeip-on.json
                         tun-on-fakeip-off.json
                         tun-off-fakeip-on.json
                         tun-off-fakeip-off.json
  AB_RUN_ID            Safe identifier: lowercase letters, digits, and hyphens

Run mode also requires SHA-256 values obtained independently from the trusted
release source:
  SING_BOX_11318_SHA256
  SING_BOX_11319_SHA256

Active modes additionally require:
  AB_CONFIRM_ISOLATED=yes
  AB_OUTPUT_DIR=/absolute/new/directory   (run mode)
  AB_CONFIRM_TRUSTED_ROOT_CODE=yes        (run mode)

The configs must use 198.18.0.2 (or AB_UPSTREAM_IP) as bootstrap DNS so all
fault traffic remains on the namespace-only veth link.
EOF
}

die() {
  echo "error: $*" >&2
  exit 2
}

refuse_default() {
  echo "refusing to touch networking: choose dry-run, verify, prepare, run, or cleanup explicitly" >&2
  usage >&2
  exit 2
}

validate_mode() {
  case "${MODE}" in
    dry-run|verify|prepare|run|cleanup) ;;
    "") refuse_default ;;
    *) die "unknown mode: ${MODE}" ;;
  esac
}

validate_run_id() {
  [[ "${RUN_ID}" =~ ^[a-z0-9][a-z0-9-]{0,23}$ ]] ||
    die "unsafe AB_RUN_ID; use 1-24 lowercase letters, digits, or hyphens"
}

validate_positive_integer() {
  local name="$1"
  local value="$2"
  [[ "${value}" =~ ^[1-9][0-9]*$ ]] || die "${name} must be a positive integer"
}

config_path() {
  local tun="$1"
  local fakeip="$2"
  printf '%s/tun-%s-fakeip-%s.json' "${CONFIG_DIR}" "${tun}" "${fakeip}"
}

validate_inputs() {
  validate_run_id
  [[ -n "${CORE_BY_VERSION[1.13.18]}" ]] || die "SING_BOX_11318_BIN must be set explicitly"
  [[ -n "${CORE_BY_VERSION[1.13.19]}" ]] || die "SING_BOX_11319_BIN must be set explicitly"
  [[ -n "${CONFIG_DIR}" && -d "${CONFIG_DIR}" ]] || die "AB_CONFIG_DIR must name an existing directory"
  [[ -f "${DNS_FIXTURE}" ]] || die "missing fixture: ${DNS_FIXTURE}"
  validate_positive_integer AB_MIXED_PORT "${MIXED_PORT}"
  (( MIXED_PORT <= 65535 )) || die "AB_MIXED_PORT must be at most 65535"
  validate_positive_integer AB_SETTLE_SECONDS "${SETTLE_SECONDS}"
  validate_positive_integer AB_PROBE_TIMEOUT "${PROBE_TIMEOUT}"
  [[ "${PROBE_URL}" == http://* || "${PROBE_URL}" == https://* ]] ||
    die "AB_PROBE_URL must use http:// or https://"
  [[ "${PROBE_URL}" != *"@"* && "${PROBE_URL}" != *"?"* && "${PROBE_URL}" != *"#"* ]] ||
    die "AB_PROBE_URL must not contain credentials, query parameters, or fragments"

  local version core tun fakeip config
  for version in "${VERSIONS[@]}"; do
    core="${CORE_BY_VERSION[${version}]}"
    [[ "${core}" == /* ]] || die "SING_BOX_${version//./}_BIN must be an absolute path"
    [[ -f "${core}" && -x "${core}" ]] || die "sing-box ${version} is not executable: ${core}"
  done
  [[ "${CORE_BY_VERSION[1.13.18]}" != "${CORE_BY_VERSION[1.13.19]}" ]] ||
    die "the two version variables must point to distinct binaries"
  for tun in "${TUN_STATES[@]}"; do
    for fakeip in "${FAKEIP_STATES[@]}"; do
      config="$(config_path "${tun}" "${fakeip}")"
      [[ -s "${config}" ]] || die "missing non-empty matrix config: ${config}"
    done
  done
}

require_core_hashes() {
  local version expected actual
  for version in "${VERSIONS[@]}"; do
    case "${version}" in
      1.13.18) expected="${EXPECTED_11318_SHA256}" ;;
      1.13.19) expected="${EXPECTED_11319_SHA256}" ;;
    esac
    [[ "${expected}" =~ ^[0-9a-fA-F]{64}$ ]] ||
      die "run requires a 64-hex SING_BOX_${version//./}_SHA256"
    actual="$(sha256sum "${CORE_BY_VERSION[${version}]}" | awk '{print $1}')"
    [[ "${actual,,}" == "${expected,,}" ]] ||
      die "sing-box ${version} SHA-256 mismatch"
  done
}

stage_trusted_cores() {
  trusted_core_dir="$(mktemp -d "/run/sboxkit-ab-${RUN_ID}-cores.XXXXXX")"
  chmod 0700 "${trusted_core_dir}"
  local version source destination expected actual
  for version in "${VERSIONS[@]}"; do
    source="${CORE_BY_VERSION[${version}]}"
    destination="${trusted_core_dir}/sing-box-${version}"
    install -m 0500 -- "${source}" "${destination}"
    case "${version}" in
      1.13.18) expected="${EXPECTED_11318_SHA256}" ;;
      1.13.19) expected="${EXPECTED_11319_SHA256}" ;;
    esac
    actual="$(sha256sum "${destination}" | awk '{print $1}')"
    [[ "${actual,,}" == "${expected,,}" ]] ||
      die "staged sing-box ${version} SHA-256 mismatch"
    CORE_BY_VERSION[${version}]="${destination}"
  done
}

cleanup_run() {
  cleanup_topology
  if [[ -n "${trusted_core_dir}" ]]; then
    case "${trusted_core_dir}" in
      "/run/sboxkit-ab-${RUN_ID}-cores."*) rm -rf -- "${trusted_core_dir}" ;;
      *) echo "refusing unsafe trusted core cleanup path: ${trusted_core_dir}" >&2 ;;
    esac
    trusted_core_dir=""
  fi
}

require_root_and_consent() {
  [[ "${EUID}" -eq 0 ]] || die "${MODE} requires root; use dry-run or verify first"
  [[ "${AB_CONFIRM_ISOLATED:-}" == "yes" ]] ||
    die "set AB_CONFIRM_ISOLATED=yes after reviewing dry-run output"
}

require_commands() {
  local command
  for command in ip nft tcpdump ss conntrack curl timeout sha256sum python3 ping install mktemp; do
    command -v "${command}" >/dev/null 2>&1 || die "required command not found: ${command}"
  done
}

verify_core() {
  local expected="$1"
  local core="${CORE_BY_VERSION[${expected}]}"
  local output
  output="$("${core}" version 2>&1)" || die "cannot execute sing-box ${expected}: ${core}"
  [[ "${output}" == *"${expected}"* ]] ||
    die "core at ${core} reports '${output}', expected ${expected}"
  echo "verified sing-box ${expected}: ${core}"
}

verify_configs_with_core() {
  local version="$1"
  local core="${CORE_BY_VERSION[${version}]}"
  local tun fakeip config
  for tun in "${TUN_STATES[@]}"; do
    for fakeip in "${FAKEIP_STATES[@]}"; do
      config="$(config_path "${tun}" "${fakeip}")"
      verify_config_shape "${config}" "${tun}" "${fakeip}"
      "${core}" check -c "${config}" >/dev/null ||
        die "sing-box ${version} rejected ${config}"
    done
  done
}

verify_config_shape() {
  local config="$1"
  local tun="$2"
  local fakeip="$3"
  command -v python3 >/dev/null 2>&1 || die "required command not found: python3"
  python3 - "${config}" "${tun}" "${fakeip}" "${UPSTREAM_IP}" <<'PY'
import json
import sys

path, expected_tun, expected_fakeip, upstream_ip = sys.argv[1:]
with open(path, "r", encoding="utf-8") as handle:
    document = json.load(handle)

inbounds = document.get("inbounds", [])
dns_servers = document.get("dns", {}).get("servers", [])
has_tun = any(isinstance(item, dict) and item.get("type") == "tun" for item in inbounds)
has_fakeip = any(
    isinstance(item, dict) and item.get("type") == "fakeip" for item in dns_servers
)
has_upstream = any(
    isinstance(item, dict) and item.get("server") == upstream_ip for item in dns_servers
)

problems = []
if has_tun != (expected_tun == "on"):
    problems.append(f"TUN presence is {has_tun}, expected {expected_tun}")
if has_fakeip != (expected_fakeip == "on"):
    problems.append(f"FakeIP presence is {has_fakeip}, expected {expected_fakeip}")
if not has_upstream:
    problems.append(f"DNS servers do not reference isolated upstream {upstream_ip}")
if problems:
    raise SystemExit(f"{path}: " + "; ".join(problems))
PY
}

verify_all() {
  local version
  for version in "${VERSIONS[@]}"; do
    verify_core "${version}"
    verify_configs_with_core "${version}"
  done
  echo "verified configuration matrix: 4 files"
}

print_matrix() {
  local version fault tun fakeip count=0
  echo "MODE=dry-run"
  echo "run_id=${RUN_ID}"
  echo "client_namespace=${CLIENT_NS}"
  echo "upstream_namespace=${UPSTREAM_NS}"
  echo "namespace_link=${CLIENT_IP}<->${UPSTREAM_IP}"
  echo "output_dir=${OUTPUT_DIR}"
  echo "host_network_changes=none (no host routes, sysctls, or firewall rules)"
  for version in "${VERSIONS[@]}"; do
    for fault in "${FAULTS[@]}"; do
      for tun in "${TUN_STATES[@]}"; do
        for fakeip in "${FAKEIP_STATES[@]}"; do
          printf 'version=%s fault=%s tun=%s fakeip=%s config=%s\n' \
            "${version}" "${fault}" "${tun}" "${fakeip}" \
            "$(config_path "${tun}" "${fakeip}")"
          count=$((count + 1))
        done
      done
    done
  done
  echo "matrix_cases=${count}"
}

namespace_exists() {
  local name="$1"
  ip netns list | awk '{print $1}' | grep -Fxq -- "${name}"
}

record_namespace_ownership() {
  local client_inode upstream_inode
  client_inode="$(stat -Lc '%i' "/var/run/netns/${CLIENT_NS}")"
  upstream_inode="$(stat -Lc '%i' "/var/run/netns/${UPSTREAM_NS}")"
  printf '%s %s\n%s %s\n' \
    "${CLIENT_NS}" "${client_inode}" "${UPSTREAM_NS}" "${upstream_inode}" >"${OWNERSHIP_FILE}"
  chmod 0600 "${OWNERSHIP_FILE}"
}

assert_owned_namespaces() {
  [[ -f "${OWNERSHIP_FILE}" ]] ||
    die "ownership marker missing; inspect namespaces and remove them manually if appropriate"
  local name expected current
  for name in "${CLIENT_NS}" "${UPSTREAM_NS}"; do
    expected="$(awk -v name="${name}" '$1 == name { print $2 }' "${OWNERSHIP_FILE}")"
    [[ "${expected}" =~ ^[0-9]+$ ]] || die "invalid ownership marker for ${name}"
    [[ -e "/var/run/netns/${name}" ]] || continue
    current="$(stat -Lc '%i' "/var/run/netns/${name}")"
    [[ "${current}" == "${expected}" ]] ||
      die "namespace ${name} does not match this harness run; refusing cleanup"
  done
}

assert_namespaces_absent() {
  ! namespace_exists "${CLIENT_NS}" || die "namespace already exists: ${CLIENT_NS}"
  ! namespace_exists "${UPSTREAM_NS}" || die "namespace already exists: ${UPSTREAM_NS}"
}

stop_processes() {
  local pid
  for pid in "${core_pid}" "${fixture_pid}" "${tcpdump_pid}"; do
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
      wait "${pid}" 2>/dev/null || true
    fi
  done
  core_pid=""
  fixture_pid=""
  tcpdump_pid=""
}

cleanup_topology() {
  stop_processes
  if [[ "${topology_active}" -eq 1 ]]; then
    ip netns delete "${CLIENT_NS}" 2>/dev/null || true
    ip netns delete "${UPSTREAM_NS}" 2>/dev/null || true
    topology_active=0
    rm -f -- "${OWNERSHIP_FILE}"
  fi
  if [[ "${veth_active}" -eq 1 && -n "${host_client_if}" ]]; then
    ip link delete "${host_client_if}" 2>/dev/null || true
    veth_active=0
  fi
}

cleanup_named_namespaces() {
  local removed=0
  assert_owned_namespaces
  if namespace_exists "${CLIENT_NS}"; then
    ip netns delete "${CLIENT_NS}"
    removed=$((removed + 1))
  fi
  if namespace_exists "${UPSTREAM_NS}"; then
    ip netns delete "${UPSTREAM_NS}"
    removed=$((removed + 1))
  fi
  rm -f -- "${OWNERSHIP_FILE}"
  echo "removed ${removed} namespace(s) for run-id ${RUN_ID}"
}

create_topology() {
  assert_namespaces_absent
  host_client_if="sbc${BASHPID}"
  host_upstream_if="sbu${BASHPID}"
  [[ ${#host_client_if} -le 15 && ${#host_upstream_if} -le 15 ]] ||
    die "temporary veth names exceed Linux IFNAMSIZ"

  ip netns add "${CLIENT_NS}"
  topology_active=1
  ip netns add "${UPSTREAM_NS}"
  record_namespace_ownership
  ip link add "${host_client_if}" type veth peer name "${host_upstream_if}"
  veth_active=1
  ip link set "${host_client_if}" netns "${CLIENT_NS}"
  ip link set "${host_upstream_if}" netns "${UPSTREAM_NS}"
  veth_active=0

  ip -n "${CLIENT_NS}" link set lo up
  ip -n "${UPSTREAM_NS}" link set lo up
  ip -n "${CLIENT_NS}" link set "${host_client_if}" name "${CLIENT_IF}"
  ip -n "${UPSTREAM_NS}" link set "${host_upstream_if}" name "${UPSTREAM_IF}"
  ip -n "${CLIENT_NS}" address add "${CLIENT_IP}/30" dev "${CLIENT_IF}"
  ip -n "${UPSTREAM_NS}" address add "${UPSTREAM_IP}/30" dev "${UPSTREAM_IF}"
  ip -n "${CLIENT_NS}" link set "${CLIENT_IF}" up
  ip -n "${UPSTREAM_NS}" link set "${UPSTREAM_IF}" up
  ip -n "${CLIENT_NS}" route add default via "${UPSTREAM_IP}" dev "${CLIENT_IF}"
}

write_snapshot() {
  local dir="$1"
  local phase="$2"
  local file="${dir}/${phase}-snapshot.txt"
  {
    echo "phase=${phase}"
    echo "captured_at=$(date --iso-8601=seconds)"
    echo "[client link]"
    ip -n "${CLIENT_NS}" -details link show
    echo "[client address]"
    ip -n "${CLIENT_NS}" address show
    echo "[client rules]"
    ip -n "${CLIENT_NS}" rule show
    echo "[client routes]"
    ip -n "${CLIENT_NS}" route show table all
    echo "[client sockets]"
    ip netns exec "${CLIENT_NS}" ss -tanup
    echo "[client conntrack]"
    ip netns exec "${CLIENT_NS}" conntrack -L
    echo "[upstream link]"
    ip -n "${UPSTREAM_NS}" -details link show
    echo "[upstream nft]"
    ip netns exec "${UPSTREAM_NS}" nft list ruleset
    echo "[resolver file]"
    sed -n '1,120p' /etc/resolv.conf
  } >"${file}" 2>&1 || true
}

apply_fault() {
  local fault="$1"
  local dir="$2"
  case "${fault}" in
    tcp-rst)
      ip netns exec "${UPSTREAM_NS}" nft add table inet sboxkit_ab
      ip netns exec "${UPSTREAM_NS}" nft add chain inet sboxkit_ab input \
        '{ type filter hook input priority 0; policy accept; }'
      ip netns exec "${UPSTREAM_NS}" nft add rule inet sboxkit_ab input \
        tcp dport 53 reject with tcp reset
      ;;
    tcp-blackhole)
      ip netns exec "${UPSTREAM_NS}" nft add table inet sboxkit_ab
      ip netns exec "${UPSTREAM_NS}" nft add chain inet sboxkit_ab input \
        '{ type filter hook input priority 0; policy accept; }'
      ip netns exec "${UPSTREAM_NS}" nft add rule inet sboxkit_ab input tcp dport 53 drop
      ;;
    dns-no-response)
      ip netns exec "${UPSTREAM_NS}" python3 "${DNS_FIXTURE}" \
        --listen "${UPSTREAM_IP}" --port 53 >"${dir}/dns-fixture.log" 2>&1 &
      fixture_pid=$!
      sleep 1
      kill -0 "${fixture_pid}" 2>/dev/null || die "DNS no-response fixture failed to start"
      ;;
    uplink-down)
      ip -n "${CLIENT_NS}" link set "${CLIENT_IF}" down
      ;;
    *) die "unsupported fault: ${fault}" ;;
  esac
}

start_capture() {
  local dir="$1"
  ip netns exec "${CLIENT_NS}" tcpdump -i any -nn -s 4096 -U \
    -w "${dir}/packets.pcap" \
    "host ${UPSTREAM_IP} and port 53" \
    >"${dir}/tcpdump.log" 2>&1 &
  tcpdump_pid=$!
  sleep 1
  kill -0 "${tcpdump_pid}" 2>/dev/null || die "tcpdump failed to start"
}

write_case_metadata() {
  local dir="$1"
  local version="$2"
  local fault="$3"
  local tun="$4"
  local fakeip="$5"
  local core="${CORE_BY_VERSION[${version}]}"
  local config
  config="$(config_path "${tun}" "${fakeip}")"
  {
    echo "run_id=${RUN_ID}"
    echo "version=${version}"
    echo "fault=${fault}"
    echo "tun=${tun}"
    echo "fakeip=${fakeip}"
    echo "core=${core}"
    echo "core_sha256=$(sha256sum "${core}" | awk '{print $1}')"
    echo "config=${config}"
    echo "config_sha256=$(sha256sum "${config}" | awk '{print $1}')"
    echo "client_namespace=${CLIENT_NS}"
    echo "upstream_namespace=${UPSTREAM_NS}"
    echo "client_ip=${CLIENT_IP}"
    echo "upstream_ip=${UPSTREAM_IP}"
    echo "probe_url=${PROBE_URL}"
    echo "mixed_port=${MIXED_PORT}"
    echo "started_at=$(date --iso-8601=seconds)"
    uname -a | sed 's/^/host_uname=/'
    "${core}" version | sed 's/^/core_version_output=/'
  } >"${dir}/metadata.txt"
}

run_probe() {
  local dir="$1"
  local result=0
  set +e
  ip netns exec "${CLIENT_NS}" timeout "${PROBE_TIMEOUT}" \
    curl --silent --show-error --output /dev/null \
    --write-out 'http_code=%{http_code} time_total=%{time_total}\n' \
    --proxy "http://127.0.0.1:${MIXED_PORT}" "${PROBE_URL}" \
    >"${dir}/probe.txt" 2>&1
  result=$?
  set -e
  echo "exit_code=${result}" >>"${dir}/probe.txt"
  return 0
}

run_case() {
  local version="$1"
  local fault="$2"
  local tun="$3"
  local fakeip="$4"
  local core="${CORE_BY_VERSION[${version}]}"
  local config dir
  config="$(config_path "${tun}" "${fakeip}")"
  dir="${OUTPUT_DIR}/version-${version}/fault-${fault}/tun-${tun}/fakeip-${fakeip}"
  mkdir -p "${dir}"

  echo "running version=${version} fault=${fault} tun=${tun} fakeip=${fakeip}"
  create_topology
  write_case_metadata "${dir}" "${version}" "${fault}" "${tun}" "${fakeip}"
  apply_fault "${fault}" "${dir}"
  start_capture "${dir}"
  write_snapshot "${dir}" before

  ip netns exec "${CLIENT_NS}" "${core}" run -c "${config}" \
    >"${dir}/sing-box.log" 2>&1 &
  core_pid=$!
  sleep "${SETTLE_SECONDS}"
  if kill -0 "${core_pid}" 2>/dev/null; then
    echo "core_started=yes" >"${dir}/result.txt"
    run_probe "${dir}"
  else
    wait "${core_pid}" 2>/dev/null || true
    core_pid=""
    echo "core_started=no" >"${dir}/result.txt"
    echo "exit_code=not-run" >"${dir}/probe.txt"
  fi
  write_snapshot "${dir}" after
  echo "finished_at=$(date --iso-8601=seconds)" >>"${dir}/result.txt"
  stop_processes
  cleanup_topology
}

write_run_manifest() {
  {
    echo "run_id=${RUN_ID}"
    echo "created_at=$(date --iso-8601=seconds)"
    echo "matrix_cases=32"
    echo "versions=${VERSIONS[*]}"
    echo "faults=${FAULTS[*]}"
    echo "tun_states=${TUN_STATES[*]}"
    echo "fakeip_states=${FAKEIP_STATES[*]}"
    echo "client_ip=${CLIENT_IP}"
    echo "upstream_ip=${UPSTREAM_IP}"
    echo "host_network_changes=none"
  } >"${OUTPUT_DIR}/manifest.txt"
}

prepare_mode() {
  require_root_and_consent
  require_commands
  assert_namespaces_absent
  trap cleanup_topology EXIT INT TERM
  create_topology
  echo "isolated topology prepared successfully"
  ip -n "${CLIENT_NS}" address show
  ip -n "${CLIENT_NS}" route show
  ip netns exec "${CLIENT_NS}" ping -c 1 -W 1 "${UPSTREAM_IP}" >/dev/null 2>&1 ||
    die "isolated veth connectivity check failed"
  echo "isolated veth connectivity verified; cleaning up"
  cleanup_topology
  trap - EXIT INT TERM
}

run_mode() {
  require_root_and_consent
  [[ "${AB_CONFIRM_TRUSTED_ROOT_CODE:-}" == "yes" ]] ||
    die "set AB_CONFIRM_TRUSTED_ROOT_CODE=yes only after verifying both core hashes and using a disposable VM"
  require_commands
  require_core_hashes
  trap cleanup_run EXIT INT TERM
  stage_trusted_cores
  verify_all
  [[ "${AB_OUTPUT_DIR:-}" == /* ]] || die "run requires an explicit absolute AB_OUTPUT_DIR"
  case "${OUTPUT_DIR}" in
    /|/home|/root|/tmp) die "AB_OUTPUT_DIR is too broad: ${OUTPUT_DIR}" ;;
  esac
  [[ ! -e "${OUTPUT_DIR}" ]] || die "AB_OUTPUT_DIR already exists; refusing to overwrite evidence"
  mkdir -p "${OUTPUT_DIR}"
  write_run_manifest

  local version fault tun fakeip
  for version in "${VERSIONS[@]}"; do
    for fault in "${FAULTS[@]}"; do
      for tun in "${TUN_STATES[@]}"; do
        for fakeip in "${FAKEIP_STATES[@]}"; do
          run_case "${version}" "${fault}" "${tun}" "${fakeip}"
        done
      done
    done
  done
  cleanup_run
  trap - EXIT INT TERM
  echo "A/B matrix complete: ${OUTPUT_DIR}"
}

main() {
  validate_mode
  if [[ "${MODE}" == "cleanup" ]]; then
    # Emergency cleanup must not depend on experiment binaries/configs still
    # being present. It remains scoped by the validated run-id and consent.
    validate_run_id
    require_root_and_consent
    for command in ip stat awk; do
      command -v "${command}" >/dev/null 2>&1 || die "required command not found: ${command}"
    done
    cleanup_named_namespaces
    return
  fi
  validate_inputs
  case "${MODE}" in
    dry-run) print_matrix ;;
    verify) verify_all ;;
    prepare) prepare_mode ;;
    run) run_mode ;;
    cleanup) die "internal mode dispatch error" ;;
  esac
}

main "$@"
