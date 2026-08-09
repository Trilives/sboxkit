#!/usr/bin/env bash
# Root-free smoke test for an extracted release archive.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
SBOXKIT_BIN="${SBOXKIT_BIN:-${SCRIPT_DIR}/sboxkit}"
export SBOXKIT_HOME="${SBOXKIT_HOME:-${SCRIPT_DIR}/.sboxkit}"

"${SBOXKIT_BIN}" version

CORE="${SBOXKIT_HOME}/bin/sing-box"
CONFIG="${SBOXKIT_HOME}/config.json"
if [[ -x "${CORE}" && -f "${CONFIG}" ]]; then
  "${CORE}" check -c "${CONFIG}"
  echo "portable config: OK"
else
  echo "portable config: skipped (initialize or run portable-update first)"
fi

echo "portable smoke test: OK ($(uname -m))"
