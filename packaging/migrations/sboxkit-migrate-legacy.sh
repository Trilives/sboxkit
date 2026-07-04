#!/usr/bin/env sh
set -eu

root="/var/lib/sboxkit"
yes=0
keep_legacy=0

usage() {
  cat <<'USAGE'
Usage: sboxkit-migrate-legacy.sh [--root DIR] [--yes] [--keep-legacy]

Converts older sboxkit state layouts into the 0.2.0 beta layout and removes
legacy heavy runtime directories after creating a backup.

Default mode is dry-run. Pass --yes to write changes.

Migrated inputs, when present:
  DIR/config.json                 -> DIR/state/config.json
  DIR/customize.json              -> DIR/state/customize.json
  DIR/active                      -> DIR/state/active
  DIR/subscriptions/              -> DIR/state/subscriptions/
  DIR/ruleset/ or DIR/rulesets/   -> DIR/state/ruleset/
  DIR/ui/                         -> DIR/state/ui/
  DIR/bin/                        -> DIR/state/bin/
  DIR/runtime/config.json         -> DIR/state/config.json if no config exists

Cleaned after backup unless --keep-legacy is set:
  DIR/runtime
  DIR/activations
  DIR/run
USAGE
}

log() {
  printf '%s\n' "$*"
}

run() {
  if [ "$yes" -eq 1 ]; then
    "$@"
  else
    printf '[dry-run] '
    printf '%s ' "$@"
    printf '\n'
  fi
}

copy_file_if_missing() {
  src="$1"
  dst="$2"
  if [ ! -f "$src" ] || [ -e "$dst" ]; then
    return 0
  fi
  run mkdir -p "$(dirname "$dst")"
  run cp -a "$src" "$dst"
}

copy_dir_if_missing() {
  src="$1"
  dst="$2"
  if [ ! -d "$src" ]; then
    return 0
  fi
  run mkdir -p "$(dirname "$dst")"
  if [ -d "$dst" ]; then
    run cp -a "$src/." "$dst/"
  else
    run cp -a "$src" "$dst"
  fi
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --root)
      root="${2:-}"; shift 2 ;;
    --root=*)
      root="${1#--root=}"; shift ;;
    --yes)
      yes=1; shift ;;
    --keep-legacy)
      keep_legacy=1; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2 ;;
  esac
done

if [ -z "$root" ]; then
  echo "--root cannot be empty" >&2
  exit 2
fi

state="$root/state"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup="$(dirname "$root")/sboxkit-migration-backup-${stamp}.tar.gz"

log "sboxkit legacy migration"
log "root: $root"
if [ "$yes" -eq 0 ]; then
  log "mode: dry-run; pass --yes to apply"
else
  log "mode: apply"
fi

if [ ! -e "$root" ]; then
  log "nothing to migrate: $root does not exist"
  exit 0
fi

if [ "$yes" -eq 1 ] && [ "$(id -u)" -ne 0 ] && [ "$root" = "/var/lib/sboxkit" ]; then
  echo "default root migration must run as root. Try: sudo ./sboxkit-migrate-legacy.sh --yes" >&2
  exit 1
fi

if [ "$yes" -eq 1 ]; then
  log "creating backup: $backup"
  tar -C "$(dirname "$root")" -czf "$backup" "$(basename "$root")"
else
  log "backup would be created at: $backup"
fi

run mkdir -p "$state" "$state/subscriptions" "$state/ruleset" "$state/ui" "$state/bin" "$root/revisions" "$root/sing-box"

copy_file_if_missing "$root/config.json" "$state/config.json"
copy_file_if_missing "$root/customize.json" "$state/customize.json"
copy_file_if_missing "$root/active" "$state/active"
copy_file_if_missing "$root/runtime/config.json" "$state/config.json"
copy_file_if_missing "$root/current/config.json" "$state/config.json"

copy_dir_if_missing "$root/subscriptions" "$state/subscriptions"
copy_dir_if_missing "$root/ruleset" "$state/ruleset"
copy_dir_if_missing "$root/rulesets" "$state/ruleset"
copy_dir_if_missing "$root/ui" "$state/ui"
copy_dir_if_missing "$root/bin" "$state/bin"

if [ "$keep_legacy" -eq 0 ]; then
  for path in "$root/runtime" "$root/activations" "$root/run"; do
    if [ -e "$path" ] || [ -L "$path" ]; then
      run rm -rf "$path"
    fi
  done
else
  log "legacy runtime directories kept because --keep-legacy was set"
fi

log "migration finished"
log "next steps:"
log "  sudo apt install ./sboxkit_0.2.0~beta_<arch>.deb"
log "  sudo sboxkit service sync"
