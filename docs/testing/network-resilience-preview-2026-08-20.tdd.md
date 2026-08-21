# v0.2.4-beta.3 TDD 与验证记录（2026-08-21）

1. Source plan

   - Derived from [todo/Todo.md](../../todo/Todo.md), focusing on the P0 items for bootstrap DNS hardening and resilience migration/state repair.

2. User journeys

   - As an existing user, I want upgraded installs to detect and repair missing or stale watchdog components, so that a soft-dead proxy can recover without manual reinstall.
   - As an operator, I want to disable network self-healing without hidden re-enablement during pause/resume, so that explicit user intent is preserved.
   - As a deployer, I want bootstrap DNS to use a direct non-looping transport with safe defaults, so that node domain resolution does not depend on the proxy itself.
   - As a user switching or pinning nodes, I want the UI to tell me which node is current, so that runtime state is visible.

3. Task report

   - Hardened bootstrap DNS config normalization and compatibility handling for older `customize.json` files.
     - RED evidence: `test: add red coverage for dns resilience and node state` on the current branch added the reproducer tests in `internal/config/config_test.go` and `internal/converter/converter_test.go`.
     - GREEN command: `go test ./internal/config ./internal/converter`
     - GREEN result excerpt: `ok github.com/Trilives/sboxkit/internal/config`, `ok github.com/Trilives/sboxkit/internal/converter`
     - Guarantee: bootstrap DoH gets safe defaults, old configs missing `bootstrap_dns_port` normalize to 443 for HTTPS, and generated bootstrap DNS does not form a resolver loop.

   - Added resilience inspection/reconcile logic for missing, stale, partially installed, or explicitly disabled watchdog state.
     - RED evidence: `test: add red coverage for dns resilience and node state` added the initial resilience reproducer tests in `internal/sysd/resilience_test.go`.
     - GREEN command: `go test ./internal/sysd`
     - GREEN result excerpt: `ok github.com/Trilives/sboxkit/internal/sysd`
     - Guarantee: existing installs can be inspected component-by-component, stale executable paths are detected, inactive timers only trigger repair when the main service is active, and invalid persisted timer intervals fall back to a safe default.

   - Made resilience preference updates transactional with systemd changes.
     - RED evidence: `internal/flows/resilience_preference_test.go` captures rollback behavior when install/remove fails.
     - GREEN command: `go test ./internal/flows`
     - GREEN result excerpt: `ok github.com/Trilives/sboxkit/internal/flows`
     - Guarantee: if privileged install/remove fails, `customize.json` reverts to the prior `enable_resilience` value instead of leaving config and systemd out of sync.

   - Removed the root-owned shared-state dependency from embedded Web panel refresh and subscription rebuild.
     - RED evidence: `ea51ed4`, `a11f837`, and `e9f52d5` reproduce runtime staging, missing access hints, and subscription rebuild failure with a read-only `state/ui`.
     - GREEN evidence: `b11be14`, `d896a68`, and `4e70d71` stage embedded assets outside shared state, always advertise the embedded panel, and stop subscription conversion from rewriting `state/ui`.
     - GREEN command: `go test ./internal/sysd ./internal/flows ./internal/subscription`
     - Guarantee: root weekly updates and regular-user rebuilds no longer change or require write access to `/var/lib/sboxkit/ui`; privileged commands only replace runtime UI.

   - Validated release packaging before preview publication.
     - Validation commands: `./scripts/fetch-deb-deps.sh`, `goreleaser release --snapshot --clean`
     - Result excerpt: `release succeeded after 4s`
     - Guarantee: the three-architecture tarball/deb pipeline still builds with the current tree once release dependencies are prefetched.

4. Test specification

   | # | What is guaranteed | Test file or command | Test type | Result | Evidence |
   |---|--------------------|----------------------|-----------|--------|----------|
   | 1 | HTTPS bootstrap without stored port normalizes to 443 | `internal/config/config_test.go:TestLoadMergesKnownFieldsWithDefaults` | unit | PASS | `go test ./internal/config` |
   | 2 | Empty DoH TLS server name falls back to a safe default | `internal/config/config_test.go:TestNormalizeBootstrapDoHRepairsEmptyTLSName` | unit | PASS | `go test ./internal/config` |
   | 3 | Bootstrap DoH is direct and has no `domain_resolver` loop | `internal/converter/converter_test.go:TestClashToSingBoxBuildsDirectBootstrapDoHWithoutResolverLoop` | integration | PASS | `go test ./internal/converter` |
   | 4 | Unused `dns-dnspod` fallback server is removed from generated config | `internal/converter/converter_test.go:TestClashToSingBoxBuildsDirectBootstrapDNSWithoutUnusedFallbackServer` | integration | PASS | `go test ./internal/converter` |
   | 5 | Paused main service does not revive watchdog timer behind the user's back | `internal/sysd/resilience_test.go:TestInspectResilienceStatusOnlyRepairsInactiveTimerWhenMainServiceIsActive` | unit | PASS | `go test ./internal/sysd` |
   | 6 | Invalid persisted watchdog interval falls back to `2min` | `internal/sysd/resilience_test.go:TestInstalledWatchdogIntervalFallsBackFromInvalidValue` | unit | PASS | `go test ./internal/sysd` |
   | 7 | `ExecStart` quotes and escapes the executable path safely | `internal/sysd/resilience_test.go:TestWatchdogServiceQuotesExecutablePath` | unit | PASS | `go test ./internal/sysd` |
   | 8 | Failed install/remove restores the prior resilience preference | `internal/flows/resilience_preference_test.go` | unit | PASS | `go test ./internal/flows` |
   | 9 | Resume does not blindly restart a residual watchdog timer | `internal/sysd/service_test.go:TestResumeCompanionUnitsExcludesResidualWatchdog` | unit | PASS | `go test ./internal/sysd` |
   | 10 | Runtime panel staging does not touch shared state UI | `internal/sysd/service_test.go:TestSyncUIRuntimeStagesEmbeddedAssetsWithoutTouchingStateUI` | unit | PASS | `go test ./internal/sysd` |
   | 11 | Subscription rebuild succeeds with read-only state UI | `internal/subscription/manager_test.go:TestRebuildDoesNotRewriteReadOnlyStateUI` | integration | PASS | `go test ./internal/subscription` |
   | 12 | Embedded panel access hint does not depend on a state UI copy | `internal/flows/menus_test.go:TestPrintAccessHintUsesEmbeddedPanelWithoutStateUICopy` | unit | PASS | `go test ./internal/flows` |

5. Coverage and known gaps

   - Full-suite command: `go test ./...`
   - Package coverage sampled during this run:
     - `go test ./internal/config -coverprofile=/tmp/config.cover.out` -> `coverage: 67.2% of statements`
     - `go test ./internal/sysd -coverprofile=/tmp/sysd.cover.out` -> `coverage: 14.4% of statements`
     - Changed helper `syncUIRuntimeWith` -> `coverage: 90.9% of statements`
   - Known gap: the repository as a whole is still below the ECC target of 80% coverage. This preview release improves coverage around the changed resilience/bootstrap paths, but it does not bring legacy packages up to 80%.

6. Merge evidence

   - RED checkpoint commit: `58394d9 test: add red coverage for dns resilience and node state`
   - GREEN checkpoint commit: `133912f feat: harden dns bootstrap and repair network resilience`
   - Final preview-release follow-up: local fixes after GREEN tightened systemd quoting/validation, resilience preference rollback, invalid interval fallback, and release snapshot verification on August 21, 2026.
   - Web panel RED checkpoints: `ea51ed4`, `a11f837`, `e9f52d5`; GREEN checkpoints: `b11be14`, `d896a68`, `4e70d71`.
