#!/usr/bin/env bash
# Update the sing-box core and geo assets for an extracted user-mode bundle and,
# when the user service is installed, deploy them into the runtime and restart it.
# User mode (sboxkit.user-mode next to the binary) only asks for sudo when TUN is
# enabled and the core changed; without a terminal the old core keeps running.
# sboxkit itself can be updated from Tools > Update.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
SBOXKIT_BIN="${SBOXKIT_BIN:-${SCRIPT_DIR}/sboxkit}"
export SBOXKIT_HOME="${SBOXKIT_HOME:-${SCRIPT_DIR}/.sboxkit}"

if [[ ! -x "${SBOXKIT_BIN}" ]]; then
  echo "sboxkit binary not found: ${SBOXKIT_BIN}" >&2
  exit 1
fi

if [[ ! -f "${SCRIPT_DIR}/sboxkit.user-mode" ]]; then
  # Without the marker `update` would manage the system service via sudo;
  # keep the historical asset-only behaviour instead.
  exec "${SBOXKIT_BIN}" assets-update
fi

exec "${SBOXKIT_BIN}" update
