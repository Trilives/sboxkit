package sysd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// resilienceLayout makes the filesystem side of the resilience installation
// inspectable without requiring tests to touch /etc.
type resilienceLayout struct {
	dispatcherDir string
	systemdDir    string
}

var defaultResilienceLayout = resilienceLayout{
	dispatcherDir: dispatcherDir,
	systemdDir:    "/etc/systemd/system",
}

func dispatcherFileIn(layout resilienceLayout, name string) string {
	return filepath.Join(layout.dispatcherDir, "90-"+name+"-restart")
}

func wdServiceIn(layout resilienceLayout) string {
	return filepath.Join(layout.systemdDir, WatchdogName+".service")
}

func wdTimerIn(layout resilienceLayout) string {
	return filepath.Join(layout.systemdDir, WatchdogName+".timer")
}

// ResilienceStatus reports each independently installed component. Stale means
// the on-disk unit differs from what this sboxkit invocation would install.
type ResilienceStatus struct {
	DispatcherSupported      bool
	DispatcherInstalled      bool
	WatchdogServiceInstalled bool
	WatchdogTimerInstalled   bool
	WatchdogTimerEnabled     bool
	WatchdogTimerActive      bool
	DispatcherStale          bool
	WatchdogServiceStale     bool
	WatchdogTimerStale       bool
	UserDisabled             bool
	NeedsRepair              bool
}

// AnyInstalled reports whether any of the three managed filesystem components
// exists, including a partial installation that should still be removable.
func (s ResilienceStatus) AnyInstalled() bool {
	return s.DispatcherInstalled || s.WatchdogServiceInstalled || s.WatchdogTimerInstalled
}

// Complete reports a healthy installation. A dispatcher is optional only on
// hosts where NetworkManager's dispatcher directory is unavailable.
func (s ResilienceStatus) Complete() bool {
	return !s.UserDisabled && !s.NeedsRepair
}

func fileMatches(path, want string) (installed, stale bool) {
	got, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	return true, !bytes.Equal(got, []byte(want))
}

func directoryExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func inspectResilienceStatus(
	layout resilienceLayout,
	name, interval, tunDev, exe string,
	timerEnabled, userDisabled bool,
	active ...bool,
) ResilienceStatus {
	dispatcherSupported := directoryExists(layout.dispatcherDir)
	dispatcherInstalled, dispatcherStale := fileMatches(
		dispatcherFileIn(layout, name), dispatcherText(name, tunDev, 20),
	)
	serviceInstalled, serviceStale := fileMatches(
		wdServiceIn(layout), wdServiceText(name, tunDev, exe),
	)
	timerInstalled, timerStale := fileMatches(wdTimerIn(layout), wdTimerText(interval))
	timerActive := timerEnabled
	if len(active) > 0 {
		timerActive = active[0]
	}

	status := ResilienceStatus{
		DispatcherSupported:      dispatcherSupported,
		DispatcherInstalled:      dispatcherInstalled,
		WatchdogServiceInstalled: serviceInstalled,
		WatchdogTimerInstalled:   timerInstalled,
		WatchdogTimerEnabled:     timerEnabled,
		WatchdogTimerActive:      timerActive,
		DispatcherStale:          dispatcherStale,
		WatchdogServiceStale:     serviceStale,
		WatchdogTimerStale:       timerStale,
		UserDisabled:             userDisabled,
	}
	status.NeedsRepair = !userDisabled && ((dispatcherSupported && (!dispatcherInstalled || dispatcherStale)) ||
		!serviceInstalled || serviceStale ||
		!timerInstalled || timerStale ||
		!timerEnabled || !timerActive)
	return status
}

func systemdUnitState(unit, state string) bool {
	result, err := exec.Command("systemctl", "is-"+state, unit).Output()
	return err == nil && strings.TrimSpace(string(result)) == state
}

// StableExecutablePath returns the path the user invoked, preserving a stable
// symlink (for example /usr/bin/sboxkit) across self-updates. os.Executable on
// Linux resolves /proc/self/exe to the current versioned target and would make
// the generated watchdog unit stale after every symlink switch.
func StableExecutablePath() (string, error) {
	return stableExecutablePath(os.Args[0], exec.LookPath)
}

func stableExecutablePath(arg0 string, lookPath func(string) (string, error)) (string, error) {
	path := arg0
	var err error
	if !strings.ContainsRune(path, os.PathSeparator) {
		path, err = lookPath(path)
		if err != nil {
			return "", err
		}
	}
	return filepath.Abs(path)
}
