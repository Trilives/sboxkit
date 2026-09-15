package sysd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestFailureClassValuesAreStableForLogsAndDiagnostics(t *testing.T) {
	tests := []struct {
		class FailureClass
		want  string
	}{
		{FailureNone, "healthy"},
		{FailurePhysicalUplink, "physical_uplink_unavailable"},
		{FailureBootstrapDNS, "bootstrap_dns_unavailable"},
		{FailureNodeResolution, "node_resolution_failed"},
		{FailureMixedPort, "mixed_port_unavailable"},
		{FailureEndToEnd, "end_to_end_failed"},
	}
	for _, test := range tests {
		if got := string(test.class); got != test.want {
			t.Errorf("failure class = %q, want stable value %q", got, test.want)
		}
	}
}

func TestWatchdogFileFailsClosedWhenPersistentStateIsCorrupted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchdog.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	decision, err := evaluateWatchdogFile(path, now, HealthReport{Failure: FailureEndToEnd}, DefaultWatchdogPolicy(), true)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Restart || decision.Reason != RestartSkippedBackoff {
		t.Fatalf("corrupted-state decision = %#v, want fail-closed backoff", decision)
	}
	if len(decision.State.RestartedAt) != DefaultWatchdogPolicy().MaxRestarts {
		t.Fatalf("synthetic restart history = %v", decision.State.RestartedAt)
	}
}

func TestMinimumUptimeSkipDoesNotConsumeRestartBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchdog.json")
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	decision, err := evaluateWatchdogFile(path, now, HealthReport{Failure: FailureMixedPort}, DefaultWatchdogPolicy(), false)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Restart || decision.Reason != RestartSkippedMinUptime {
		t.Fatalf("minimum-uptime decision = %#v", decision)
	}
	if len(decision.State.RestartedAt) != 0 || decision.State.RestartAttempts != 0 {
		t.Fatalf("minimum-uptime skip consumed budget: %#v", decision.State)
	}
	if !decision.CaptureSnapshot {
		t.Fatal("minimum-uptime skip lost first-failure snapshot request")
	}
}

func TestHealthRuntimePathsHonorOverride(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("SBOXKIT_RUN_DIR", directory)
	if got := HealthRuntimeDir(); got != directory {
		t.Fatalf("HealthRuntimeDir() = %q, want %q", got, directory)
	}
	if got := DefaultWatchdogStatePath(); got != filepath.Join(directory, "watchdog-state.json") {
		t.Fatalf("state path = %q", got)
	}
	if got := DefaultDiagnosticSnapshotPath(); got != filepath.Join(directory, "health-diagnostic.json") {
		t.Fatalf("snapshot path = %q", got)
	}
}

func TestHealthcheckRunLockRejectsConcurrentInvocationWithoutWaiting(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("SBOXKIT_RUN_DIR", directory)
	release, err := acquireHealthcheckRunLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	secondRelease, err := acquireHealthcheckRunLock()
	if secondRelease != nil {
		secondRelease()
	}
	if !errors.Is(err, errHealthcheckAlreadyRunning) {
		t.Fatalf("concurrent lock error = %v, want errHealthcheckAlreadyRunning", err)
	}
}

func TestSnapshotCaptureCanBeRetriedAfterPersistenceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchdog.json")
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	first := EvaluateWatchdog(now, WatchdogState{}, HealthReport{Failure: FailureBootstrapDNS}, DefaultWatchdogPolicy())
	if !first.CaptureSnapshot || !first.ReplaceSnapshot {
		t.Fatal("first failure did not request replacement of any orphan snapshot")
	}
	if first.State.SnapshotCaptured {
		t.Fatal("snapshot marked captured before durable snapshot persistence")
	}
	if err := SaveWatchdogState(path, first.State); err != nil {
		t.Fatal(err)
	}
	state, err := LoadWatchdogState(path)
	if err != nil {
		t.Fatal(err)
	}
	retry := EvaluateWatchdog(now.Add(30*time.Second), state, HealthReport{Failure: FailureBootstrapDNS}, DefaultWatchdogPolicy())
	if !retry.CaptureSnapshot {
		t.Fatalf("snapshot persistence failure was not retried: %#v", retry)
	}
	if err := setWatchdogSnapshotCaptured(path, true); err != nil {
		t.Fatal(err)
	}
	persisted, err := LoadWatchdogState(path)
	if err != nil {
		t.Fatal(err)
	}
	if again := EvaluateWatchdog(now.Add(45*time.Second), persisted, HealthReport{Failure: FailureBootstrapDNS}, DefaultWatchdogPolicy()); again.CaptureSnapshot {
		t.Fatalf("durably captured snapshot requested again: %#v", again)
	}
}

func TestRecoveryPersistsSnapshotStaleUntilCleanupCompletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchdog.json")
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	state := WatchdogState{InFailure: true, SnapshotCaptured: true, ConsecutiveHealthy: 1}
	if err := SaveWatchdogState(path, state); err != nil {
		t.Fatal(err)
	}
	decision, err := evaluateWatchdogFile(path, now, HealthReport{Healthy: true}, DefaultWatchdogPolicy(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Recovered {
		t.Fatalf("decision = %#v, want pending recovery", decision)
	}
	pending, err := LoadWatchdogState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !pending.InFailure || !pending.SnapshotStale {
		t.Fatalf("persisted recovery state = %#v, want stale snapshot marker", pending)
	}
	if err := completeWatchdogRecovery(path); err != nil {
		t.Fatal(err)
	}
	completed, err := LoadWatchdogState(path)
	if err != nil {
		t.Fatal(err)
	}
	if completed.InFailure || completed.SnapshotStale {
		t.Fatalf("completed recovery state = %#v", completed)
	}
}

func TestFailureDuringPendingRecoveryReplacesStaleSnapshot(t *testing.T) {
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	state := WatchdogState{InFailure: true, SnapshotCaptured: true, SnapshotStale: true, ConsecutiveHealthy: 2}
	decision := EvaluateWatchdog(now, state, HealthReport{Failure: FailureBootstrapDNS}, DefaultWatchdogPolicy())
	if !decision.CaptureSnapshot || !decision.ReplaceSnapshot {
		t.Fatalf("decision = %#v, want stale snapshot replacement", decision)
	}
}

func TestRestartSkipReasonValuesAreStableForLogs(t *testing.T) {
	tests := []struct {
		reason RestartReason
		want   string
	}{
		{RestartSkippedBackoff, "backoff"},
		{RestartSkippedRateLimit, "rate_limit"},
		{RestartSkippedNoUplink, "no_physical_uplink"},
	}
	for _, test := range tests {
		if got := string(test.reason); got != test.want {
			t.Errorf("restart reason = %q, want stable value %q", got, test.want)
		}
	}
}

func TestRunLayeredProbesClassifiesFailuresAndShortCircuits(t *testing.T) {
	layers := []ProbeLayer{
		ProbePhysicalUplink,
		ProbeBootstrapDNS,
		ProbeNodeResolution,
		ProbeMixedPort,
		ProbeEndToEnd,
	}
	wantClasses := []FailureClass{
		FailurePhysicalUplink,
		FailureBootstrapDNS,
		FailureNodeResolution,
		FailureMixedPort,
		FailureEndToEnd,
	}

	for failedAt, wantClass := range wantClasses {
		failedAt, wantClass := failedAt, wantClass
		t.Run(string(wantClass), func(t *testing.T) {
			calls := make([]ProbeLayer, 0, len(layers))
			probes := make([]HealthProbe, 0, len(layers))
			for index, layer := range layers {
				index, layer := index, layer
				probes = append(probes, HealthProbe{
					Layer: layer,
					Run: func() error {
						calls = append(calls, layer)
						if index == failedAt {
							return errors.New("injected probe failure")
						}
						return nil
					},
				})
			}

			report := RunLayeredProbes(probes)
			if report.Healthy {
				t.Fatal("report is healthy after an injected failure")
			}
			if report.Failure != wantClass {
				t.Fatalf("failure class = %q, want %q", report.Failure, wantClass)
			}
			wantCalls := layers[:failedAt+1]
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("probe calls = %v, want short-circuit sequence %v", calls, wantCalls)
			}
			if len(report.Results) != failedAt+1 {
				t.Fatalf("result count = %d, want %d", len(report.Results), failedAt+1)
			}
			for index, result := range report.Results {
				if result.Layer != layers[index] {
					t.Fatalf("result[%d].Layer = %q, want %q", index, result.Layer, layers[index])
				}
				wantOK := index != failedAt
				if result.OK != wantOK {
					t.Fatalf("result[%d].OK = %v, want %v", index, result.OK, wantOK)
				}
			}
		})
	}
}

func TestRunLayeredProbesReportsAllHealthyLayers(t *testing.T) {
	probes := []HealthProbe{
		{Layer: ProbePhysicalUplink, Run: func() error { return nil }},
		{Layer: ProbeBootstrapDNS, Run: func() error { return nil }},
		{Layer: ProbeNodeResolution, Run: func() error { return nil }},
		{Layer: ProbeMixedPort, Run: func() error { return nil }},
		{Layer: ProbeEndToEnd, Run: func() error { return nil }},
	}

	report := RunLayeredProbes(probes)
	if !report.Healthy || report.Failure != FailureNone {
		t.Fatalf("healthy report = %#v", report)
	}
	if len(report.Results) != len(probes) {
		t.Fatalf("result count = %d, want %d", len(report.Results), len(probes))
	}
}

func TestWatchdogStateSurvivesSeparateInvocations(t *testing.T) {
	policy := WatchdogPolicy{
		Window:           10 * time.Minute,
		MaxRestarts:      2,
		InitialBackoff:   time.Minute,
		MaxBackoff:       8 * time.Minute,
		HealthyThreshold: 2,
	}
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	failure := HealthReport{Failure: FailureBootstrapDNS}

	first := EvaluateWatchdog(now, WatchdogState{}, failure, policy)
	if !first.Restart || !first.CaptureSnapshot {
		t.Fatalf("first failure decision = %#v, want restart and snapshot", first)
	}
	if first.State.NextRestartAt != now.Add(time.Minute) {
		t.Fatalf("next restart = %v, want %v", first.State.NextRestartAt, now.Add(time.Minute))
	}
	first.State.SnapshotCaptured = true // simulate successful snapshot write
	first.State.SnapshotStale = false

	statePath := filepath.Join(t.TempDir(), "watchdog-state.json")
	if err := SaveWatchdogState(statePath, first.State); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadWatchdogState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, first.State) {
		t.Fatalf("loaded state = %#v, want %#v", loaded, first.State)
	}

	tooSoon := EvaluateWatchdog(now.Add(30*time.Second), loaded, failure, policy)
	if tooSoon.Restart || tooSoon.Reason != RestartSkippedBackoff {
		t.Fatalf("decision during backoff = %#v", tooSoon)
	}
	if tooSoon.CaptureSnapshot {
		t.Fatal("a later invocation must not request another first-failure snapshot")
	}
}

func TestWatchdogAllowsAtMostTwoRestartsInAnyTenMinutes(t *testing.T) {
	policy := WatchdogPolicy{
		Window:           10 * time.Minute,
		MaxRestarts:      2,
		InitialBackoff:   time.Minute,
		MaxBackoff:       8 * time.Minute,
		HealthyThreshold: 2,
	}
	start := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	failure := HealthReport{Failure: FailureBootstrapDNS}

	first := EvaluateWatchdog(start, WatchdogState{}, failure, policy)
	second := EvaluateWatchdog(start.Add(time.Minute), first.State, failure, policy)
	third := EvaluateWatchdog(start.Add(3*time.Minute), second.State, failure, policy)

	if !first.Restart || !second.Restart {
		t.Fatalf("first two eligible failures must restart: first=%#v second=%#v", first, second)
	}
	if third.Restart || third.Reason != RestartSkippedRateLimit {
		t.Fatalf("third restart inside ten minutes = %#v, want rate-limited", third)
	}

	afterWindow := EvaluateWatchdog(start.Add(10*time.Minute+time.Second), third.State, failure, policy)
	if !afterWindow.Restart {
		t.Fatalf("restart after sliding window expiry = %#v, want allowed", afterWindow)
	}
}

func TestWatchdogDoesNotRestartForMissingPhysicalUplink(t *testing.T) {
	policy := WatchdogPolicy{
		Window:           10 * time.Minute,
		MaxRestarts:      2,
		InitialBackoff:   time.Minute,
		MaxBackoff:       8 * time.Minute,
		HealthyThreshold: 2,
	}
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)

	decision := EvaluateWatchdog(now, WatchdogState{}, HealthReport{Failure: FailurePhysicalUplink}, policy)
	if decision.Restart || decision.Reason != RestartSkippedNoUplink {
		t.Fatalf("missing-uplink decision = %#v, want an explicit no-uplink skip", decision)
	}
	if !decision.CaptureSnapshot {
		t.Fatal("the first no-uplink failure should still preserve diagnostic evidence")
	}
}

func TestWatchdogBackoffGrowsExponentiallyAndCaps(t *testing.T) {
	policy := WatchdogPolicy{
		Window:           time.Hour,
		MaxRestarts:      10,
		InitialBackoff:   time.Minute,
		MaxBackoff:       4 * time.Minute,
		HealthyThreshold: 2,
	}
	failure := HealthReport{Failure: FailureEndToEnd}
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	state := WatchdogState{}

	wantDelays := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 4 * time.Minute}
	for index, wantDelay := range wantDelays {
		decision := EvaluateWatchdog(now, state, failure, policy)
		if !decision.Restart {
			t.Fatalf("restart %d = %#v, want allowed", index+1, decision)
		}
		if got := decision.State.NextRestartAt.Sub(now); got != wantDelay {
			t.Fatalf("restart %d backoff = %v, want %v", index+1, got, wantDelay)
		}
		state = decision.State
		now = decision.State.NextRestartAt
	}
}

func TestWatchdogRequiresTwoConsecutiveHealthyReportsBeforeReset(t *testing.T) {
	policy := WatchdogPolicy{
		Window:           10 * time.Minute,
		MaxRestarts:      2,
		InitialBackoff:   time.Minute,
		MaxBackoff:       8 * time.Minute,
		HealthyThreshold: 2,
	}
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	failure := HealthReport{Failure: FailureMixedPort}
	healthy := HealthReport{Healthy: true, Failure: FailureNone}

	failed := EvaluateWatchdog(now, WatchdogState{}, failure, policy)
	failed.State.SnapshotCaptured = true // simulate successful snapshot write
	failed.State.SnapshotStale = false
	oneHealthy := EvaluateWatchdog(now.Add(2*time.Minute), failed.State, healthy, policy)
	if oneHealthy.Recovered {
		t.Fatal("one healthy report must not mark the watchdog recovered")
	}
	if !oneHealthy.State.InFailure || !oneHealthy.State.SnapshotCaptured {
		t.Fatalf("one healthy report cleared failure evidence: %#v", oneHealthy.State)
	}

	twoHealthy := EvaluateWatchdog(now.Add(4*time.Minute), oneHealthy.State, healthy, policy)
	if !twoHealthy.Recovered {
		t.Fatalf("second consecutive healthy report = %#v, want recovery", twoHealthy)
	}
	if twoHealthy.State.InFailure || twoHealthy.State.SnapshotCaptured || len(twoHealthy.State.RestartedAt) != 0 {
		t.Fatalf("recovered state retained stale failure data: %#v", twoHealthy.State)
	}

	again := EvaluateWatchdog(now.Add(6*time.Minute), twoHealthy.State, failure, policy)
	if !again.CaptureSnapshot {
		t.Fatal("a new failure episode must capture a new first-failure snapshot")
	}
}

func TestWatchdogFailureInterruptsRecoveryStreak(t *testing.T) {
	policy := WatchdogPolicy{
		Window:           10 * time.Minute,
		MaxRestarts:      2,
		InitialBackoff:   time.Minute,
		MaxBackoff:       8 * time.Minute,
		HealthyThreshold: 2,
	}
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	failure := HealthReport{Failure: FailureEndToEnd}
	healthy := HealthReport{Healthy: true, Failure: FailureNone}

	failed := EvaluateWatchdog(now, WatchdogState{}, failure, policy)
	failed.State.SnapshotCaptured = true // simulate successful snapshot write
	failed.State.SnapshotStale = false
	oneHealthy := EvaluateWatchdog(now.Add(time.Minute), failed.State, healthy, policy)
	failedAgain := EvaluateWatchdog(now.Add(2*time.Minute), oneHealthy.State, failure, policy)
	healthyAgain := EvaluateWatchdog(now.Add(3*time.Minute), failedAgain.State, healthy, policy)

	if healthyAgain.Recovered {
		t.Fatal("healthy, failed, healthy reports are not two consecutive healthy rounds")
	}
	if !healthyAgain.State.InFailure {
		t.Fatalf("interrupted recovery streak cleared failure state: %#v", healthyAgain.State)
	}
}

func TestWatchdogEvaluationDoesNotMutateCallerState(t *testing.T) {
	now := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	original := WatchdogState{
		InFailure:        true,
		SnapshotCaptured: true,
		RestartedAt:      []time.Time{now.Add(-time.Minute)},
		NextRestartAt:    now,
	}
	want := WatchdogState{
		InFailure:        original.InFailure,
		SnapshotCaptured: original.SnapshotCaptured,
		RestartedAt:      append([]time.Time(nil), original.RestartedAt...),
		NextRestartAt:    original.NextRestartAt,
	}

	_ = EvaluateWatchdog(now, original, HealthReport{Failure: FailureEndToEnd}, WatchdogPolicy{
		Window:           10 * time.Minute,
		MaxRestarts:      2,
		InitialBackoff:   time.Minute,
		MaxBackoff:       8 * time.Minute,
		HealthyThreshold: 2,
	})

	if !reflect.DeepEqual(original, want) {
		t.Fatalf("EvaluateWatchdog mutated caller state: got %#v, want %#v", original, want)
	}
}
