#!/usr/bin/env bash
# Root-free smoke test for an extracted user-mode bundle.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
SBOXKIT_BIN="${SBOXKIT_BIN:-${SCRIPT_DIR}/sboxkit}"
export SBOXKIT_HOME="${SBOXKIT_HOME:-${SCRIPT_DIR}/.sboxkit}"

"${SBOXKIT_BIN}" version

BUNDLED_CORE="${SCRIPT_DIR}/core/sing-box"
if [[ -x "${BUNDLED_CORE}" ]]; then
  "${BUNDLED_CORE}" version | head -n 1
  echo "bundled core: OK"
else
  echo "bundled core: missing (a download will be needed during init)"
fi

CORE="${SBOXKIT_HOME}/bin/sing-box"
CONFIG="${SBOXKIT_HOME}/config.json"
if [[ -x "${CORE}" && -f "${CONFIG}" ]]; then
  "${CORE}" check -c "${CONFIG}"
  echo "user-mode config: OK"
else
  echo "user-mode config: skipped (run ./sboxkit to initialize first)"
fi

if command -v systemctl >/dev/null 2>&1; then
  state="$(systemctl --user is-active sing-box.service 2>/dev/null || true)"
  echo "user service sing-box.service: ${state:-unknown}"
fi

echo "user-mode smoke test: OK ($(uname -m))"
