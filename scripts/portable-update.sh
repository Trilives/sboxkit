#!/usr/bin/env bash
# Update the sing-box core and geo assets for an extracted sboxkit archive.
# sboxkit itself can be updated from Tools > Update without root when this
# directory is writable.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
SBOXKIT_BIN="${SBOXKIT_BIN:-${SCRIPT_DIR}/sboxkit}"
export SBOXKIT_HOME="${SBOXKIT_HOME:-${SCRIPT_DIR}/.sboxkit}"

if [[ ! -x "${SBOXKIT_BIN}" ]]; then
  echo "sboxkit binary not found: ${SBOXKIT_BIN}" >&2
  exit 1
fi

exec "${SBOXKIT_BIN}" portable-update
