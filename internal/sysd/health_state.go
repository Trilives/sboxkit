package sysd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const defaultHealthRuntimeDir = "/run/sboxkit"

var errHealthcheckAlreadyRunning = errors.New("healthcheck is already running")

type RestartReason string

const (
	RestartSkippedBackoff   RestartReason = "backoff"
	RestartSkippedRateLimit RestartReason = "rate_limit"
	RestartSkippedNoUplink  RestartReason = "no_physical_uplink"
	RestartSkippedMinUptime RestartReason = "minimum_uptime"
)

type WatchdogPolicy struct {
	Window           time.Duration
	MaxRestarts      int
	InitialBackoff   time.Duration
	MaxBackoff       time.Duration
	HealthyThreshold int
}

type WatchdogState struct {
	InFailure          bool         `json:"in_failure"`
	SnapshotCaptured   bool         `json:"snapshot_captured"`
	SnapshotStale      bool         `json:"snapshot_stale,omitempty"`
	RestartedAt        []time.Time  `json:"restarted_at,omitempty"`
	NextRestartAt      time.Time    `json:"next_restart_at,omitempty"`
	ConsecutiveHealthy int          `json:"consecutive_healthy,omitempty"`
	RestartAttempts    int          `json:"restart_attempts,omitempty"`
	LastFailure        FailureClass `json:"last_failure,omitempty"`
}

type WatchdogDecision struct {
	State           WatchdogState
	Restart         bool
	Reason          RestartReason
	CaptureSnapshot bool
	ReplaceSnapshot bool
	Recovered       bool
}

func DefaultWatchdogPolicy() WatchdogPolicy {
	return WatchdogPolicy{
		Window: 10 * time.Minute, MaxRestarts: 2,
		InitialBackoff: time.Minute, MaxBackoff: 8 * time.Minute,
		HealthyThreshold: 2,
	}
}

func HealthRuntimeDir() string {
	if value := os.Getenv("SBOXKIT_RUN_DIR"); value != "" {
		return value
	}
	return defaultHealthRuntimeDir
}

func DefaultWatchdogStatePath() string {
	return filepath.Join(HealthRuntimeDir(), "watchdog-state.json")
}

func DefaultDiagnosticSnapshotPath() string {
	return filepath.Join(HealthRuntimeDir(), "health-diagnostic.json")
}

// EvaluateWatchdog is pure with respect to caller-owned state. This matters
// both to tests and to callers which need to retain the state preceding an
// unsuccessful systemctl restart.
func EvaluateWatchdog(now time.Time, previous WatchdogState, report HealthReport, policy WatchdogPolicy) WatchdogDecision {
	policy = normalizedPolicy(policy)
	state := cloneWatchdogState(previous)
	state.RestartedAt = recentRestarts(now, state.RestartedAt, policy.Window)

	if report.Healthy {
		if !state.InFailure {
			return WatchdogDecision{State: state}
		}
		state.ConsecutiveHealthy++
		if state.ConsecutiveHealthy < policy.HealthyThreshold {
			return WatchdogDecision{State: state}
		}
		return WatchdogDecision{State: WatchdogState{}, Recovered: true}
	}

	decision := WatchdogDecision{State: state}
	decision.State.ConsecutiveHealthy = 0
	decision.State.LastFailure = report.Failure
	if !decision.State.InFailure {
		decision.State.InFailure = true
		// Any snapshot found beside an empty state belongs to an interrupted
		// recovery or an older implementation. Treat it as stale and replace it
		// before capturing the first evidence for this new incident.
		decision.State.SnapshotCaptured = false
		decision.State.SnapshotStale = true
	}
	if decision.State.SnapshotStale {
		decision.State.SnapshotCaptured = false
		decision.CaptureSnapshot = true
		decision.ReplaceSnapshot = true
	} else if !decision.State.SnapshotCaptured {
		decision.CaptureSnapshot = true
	}
	if report.Failure == FailurePhysicalUplink {
		decision.Reason = RestartSkippedNoUplink
		return decision
	}
	if !decision.State.NextRestartAt.IsZero() && now.Before(decision.State.NextRestartAt) {
		decision.Reason = RestartSkippedBackoff
		return decision
	}
	if len(decision.State.RestartedAt) >= policy.MaxRestarts {
		decision.Reason = RestartSkippedRateLimit
		return decision
	}

	delay := exponentialBackoff(policy.InitialBackoff, policy.MaxBackoff, decision.State.RestartAttempts)
	decision.State.RestartAttempts++
	decision.State.RestartedAt = append(decision.State.RestartedAt, now)
	decision.State.NextRestartAt = now.Add(delay)
	decision.Restart = true
	return decision
}

func normalizedPolicy(policy WatchdogPolicy) WatchdogPolicy {
	defaults := DefaultWatchdogPolicy()
	if policy.Window <= 0 {
		policy.Window = defaults.Window
	}
	if policy.MaxRestarts <= 0 {
		policy.MaxRestarts = defaults.MaxRestarts
	}
	if policy.InitialBackoff <= 0 {
		policy.InitialBackoff = defaults.InitialBackoff
	}
	if policy.MaxBackoff < policy.InitialBackoff {
		policy.MaxBackoff = defaults.MaxBackoff
	}
	if policy.HealthyThreshold <= 0 {
		policy.HealthyThreshold = defaults.HealthyThreshold
	}
	return policy
}

func exponentialBackoff(initial, maximum time.Duration, exponent int) time.Duration {
	delay := initial
	for index := 0; index < exponent && delay < maximum; index++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func recentRestarts(now time.Time, restarts []time.Time, window time.Duration) []time.Time {
	cutoff := now.Add(-window)
	recent := make([]time.Time, 0, len(restarts))
	for _, restartedAt := range restarts {
		if restartedAt.After(cutoff) && !restartedAt.After(now) {
			recent = append(recent, restartedAt)
		}
	}
	return recent
}

func cloneWatchdogState(state WatchdogState) WatchdogState {
	state.RestartedAt = append([]time.Time(nil), state.RestartedAt...)
	return state
}

func LoadWatchdogState(path string) (WatchdogState, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return WatchdogState{}, nil
	}
	if err != nil {
		return WatchdogState{}, fmt.Errorf("read watchdog state: %w", err)
	}
	var state WatchdogState
	if err := json.Unmarshal(raw, &state); err != nil {
		return WatchdogState{}, fmt.Errorf("decode watchdog state: %w", err)
	}
	return state, nil
}

func SaveWatchdogState(path string, state WatchdogState) error {
	return withHealthFileLock(path, func() error { return saveWatchdogStateUnlocked(path, state) })
}

func setWatchdogSnapshotCaptured(path string, captured bool) error {
	return withHealthFileLock(path, func() error {
		state, err := LoadWatchdogState(path)
		if err != nil {
			return err
		}
		state.SnapshotCaptured = captured
		if captured {
			state.SnapshotStale = false
		}
		return saveWatchdogStateUnlocked(path, state)
	})
}

func completeWatchdogRecovery(path string) error {
	return SaveWatchdogState(path, WatchdogState{})
}

func evaluateWatchdogFile(path string, now time.Time, report HealthReport, policy WatchdogPolicy, restartAllowed bool) (WatchdogDecision, error) {
	var decision WatchdogDecision
	err := withHealthFileLock(path, func() error {
		state, err := LoadWatchdogState(path)
		if err != nil {
			// A damaged state file must fail closed. Replacing it with a full
			// window of synthetic attempts prevents corruption from resetting
			// the limiter and causing an immediate restart storm.
			policy = normalizedPolicy(policy)
			state = WatchdogState{
				InFailure:       true,
				RestartedAt:     make([]time.Time, policy.MaxRestarts),
				NextRestartAt:   now.Add(policy.Window),
				RestartAttempts: policy.MaxRestarts,
			}
			for index := range state.RestartedAt {
				state.RestartedAt[index] = now
			}
		}
		previous := cloneWatchdogState(state)
		decision = EvaluateWatchdog(now, state, report, policy)
		if decision.Restart && !restartAllowed {
			decision.Restart = false
			decision.Reason = RestartSkippedMinUptime
			decision.State.RestartedAt = recentRestarts(now, previous.RestartedAt, normalizedPolicy(policy).Window)
			decision.State.RestartAttempts = previous.RestartAttempts
			decision.State.NextRestartAt = previous.NextRestartAt
		}
		stateToPersist := decision.State
		if decision.Recovered {
			// Keep an explicit transition state until the old snapshot has been
			// removed. This makes a crash between the two file operations
			// recoverable and prevents old evidence from becoming the next
			// incident's supposedly first snapshot.
			stateToPersist = previous
			stateToPersist.ConsecutiveHealthy = normalizedPolicy(policy).HealthyThreshold
			stateToPersist.SnapshotStale = true
		}
		return saveWatchdogStateUnlocked(path, stateToPersist)
	})
	return decision, err
}

func saveWatchdogStateUnlocked(path string, state WatchdogState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode watchdog state: %w", err)
	}
	// State contains only classifications and timestamps. World-readable mode
	// lets the unprivileged `sboxkit diagnose` command inspect root timer state;
	// only root can replace files in the 0755-owned runtime directory.
	return atomicWriteFile(path, append(raw, '\n'), 0o644)
}

func withHealthFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create health state directory: %w", err)
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open health state lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock health state: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

func acquireHealthcheckRunLock() (func(), error) {
	if err := os.MkdirAll(HealthRuntimeDir(), 0o755); err != nil {
		return nil, fmt.Errorf("create health runtime directory: %w", err)
	}
	path := filepath.Join(HealthRuntimeDir(), "healthcheck-run.lock")
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open healthcheck run lock: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errHealthcheckAlreadyRunning
		}
		return nil, fmt.Errorf("lock healthcheck run: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}, nil
}

func atomicWriteFile(path string, contents []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
