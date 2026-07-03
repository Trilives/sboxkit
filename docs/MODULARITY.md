# Modularity Constraints

This project should stay small enough to reason about from the terminal. New
work should keep behavior grouped by domain and avoid single-file growth.

## File Size

- Prefer files under 300 lines.
- Files over 400 lines need a clear reason and should be split before adding
  unrelated behavior.
- Files over 600 lines are considered a refactor target.

## Package Boundaries

- `internal/tui` owns terminal rendering, key handling, text input, confirmation,
  width handling, and non-TTY fallback.
- `internal/app` owns command dispatch and flow composition. It should call TUI
  primitives but must not implement raw terminal rendering.
- `internal/system` owns systemd, firewall, timers, and OS command execution.
- `internal/subscription` owns subscription storage, fetching, detection, and
  conversion entry points.
- `internal/converter` owns config conversion only. It should not fetch network
  assets or run system commands.
- `internal/download` owns external asset downloads and cache updates.

## TUI Rules

- Add new interactive screens as focused flow files, for example
  `tui_config.go`, `tui_subscription.go`, or `tui_system.go`.
- Use `internal/tui.Select`, `Ask`, `Confirm`, and `Pause`; do not add new
  ad-hoc raw terminal code in `internal/app`.
- Put related toggles under submenus instead of growing a flat top-level list.
- Keep business operations callable from CLI commands first, then wire the TUI
  to those commands.

## Change Rules

- When a feature crosses package boundaries, add tests at the smallest practical
  level first.
- Keep package APIs narrow. Export only what another package actually needs.
- Prefer moving code into a cohesive package over adding another helper block to
  an already large file.
