package flows

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
)

// DiagnoseOptions controls the presentation and injectable inputs of the
// read-only diagnostic view. Empty paths select the watchdog's standard files.
type DiagnoseOptions struct {
	Writer       io.Writer
	JSON         bool
	StatePath    string
	SnapshotPath string
}

// Diagnose reads previously captured watchdog evidence. It deliberately does
// not create state directories, invoke system commands, or repair services.
func Diagnose(_ paths.Paths, options DiagnoseOptions) error {
	writer := options.Writer
	if writer == nil {
		writer = os.Stdout
	}
	statePath := options.StatePath
	if statePath == "" {
		statePath = sysd.DefaultWatchdogStatePath()
	}
	snapshotPath := options.SnapshotPath
	if snapshotPath == "" {
		snapshotPath = sysd.DefaultDiagnosticSnapshotPath()
	}

	state, err := readDiagnosticFile[sysd.WatchdogState](statePath)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("无法读取 watchdog 状态"), err)
	}
	snapshot, err := readDiagnosticFile[sysd.DiagnosticSnapshot](snapshotPath)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("无法读取诊断快照"), err)
	}
	return renderDiagnostic(writer, state, snapshot, options.JSON)
}

func readDiagnosticFile[T any](path string) (*T, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

type diagnosticJSON struct {
	Status   diagnosticStatusJSON   `json:"status"`
	Snapshot diagnosticSnapshotJSON `json:"snapshot"`
}

type diagnosticStatusJSON struct {
	Available     bool   `json:"available"`
	InFailure     bool   `json:"in_failure"`
	RestartCount  int    `json:"restart_count"`
	NextRestartAt string `json:"next_restart_at"`
}

type diagnosticSnapshotJSON struct {
	Available        bool                  `json:"available"`
	GeneratedAt      string                `json:"generated_at,omitempty"`
	Failure          string                `json:"failure,omitempty"`
	DefaultInterface string                `json:"default_interface,omitempty"`
	BootstrapTargets []string              `json:"bootstrap_targets,omitempty"`
	Probes           []diagnosticProbeJSON `json:"probes,omitempty"`
	Sections         map[string]string     `json:"sections,omitempty"`
}

type diagnosticProbeJSON struct {
	Layer  string `json:"layer"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

func renderDiagnostic(writer io.Writer, state *sysd.WatchdogState, snapshot *sysd.DiagnosticSnapshot, jsonOutput bool) error {
	if jsonOutput {
		return renderDiagnosticJSON(writer, state, snapshot)
	}
	return renderDiagnosticText(writer, state, snapshot)
}

func renderDiagnosticJSON(writer io.Writer, state *sysd.WatchdogState, snapshot *sysd.DiagnosticSnapshot) error {
	view := diagnosticJSON{
		Status: diagnosticStatusJSON{Available: state != nil},
		Snapshot: diagnosticSnapshotJSON{
			Available: snapshot != nil,
		},
	}
	if state != nil {
		view.Status.InFailure = state.InFailure
		view.Status.RestartCount = len(state.RestartedAt)
		view.Status.NextRestartAt = formatDiagnosticTime(state.NextRestartAt)
	}
	if snapshot != nil {
		probes := make([]diagnosticProbeJSON, len(snapshot.ProbeResults))
		for index, probe := range snapshot.ProbeResults {
			probes[index] = diagnosticProbeJSON{
				Layer:  string(probe.Layer),
				OK:     probe.OK,
				Detail: probe.Detail,
			}
		}
		view.Snapshot.GeneratedAt = formatDiagnosticTime(snapshot.GeneratedAt)
		view.Snapshot.Failure = diagnosticFailureName(snapshot.Failure)
		view.Snapshot.DefaultInterface = snapshot.DefaultInterface
		view.Snapshot.BootstrapTargets = append([]string(nil), snapshot.BootstrapTargets...)
		view.Snapshot.Probes = probes
		if len(snapshot.Sections) > 0 {
			view.Snapshot.Sections = make(map[string]string, len(snapshot.Sections))
			for key, value := range snapshot.Sections {
				view.Snapshot.Sections[key] = value
			}
		}
	}

	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(view)
}

func renderDiagnosticText(writer io.Writer, state *sysd.WatchdogState, snapshot *sysd.DiagnosticSnapshot) error {
	if _, err := fmt.Fprintln(writer, i18n.T("故障诊断")); err != nil {
		return err
	}
	if state == nil {
		if _, err := fmt.Fprintln(writer, i18n.T("watchdog 状态：不可用（尚未记录状态）")); err != nil {
			return err
		}
	} else {
		status := i18n.T("健康")
		if state.InFailure {
			failure := "unknown"
			if snapshot != nil && snapshot.Failure != "" {
				failure = diagnosticFailureName(snapshot.Failure)
			}
			status = fmt.Sprintf(i18n.T("故障（%s）"), failure)
		}
		if _, err := fmt.Fprintf(writer, "%s: %s\n", i18n.T("状态"), status); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "%s: %d\n", i18n.T("重启尝试次数"), len(state.RestartedAt)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "%s: %s\n", i18n.T("下次允许重启"), formatDiagnosticTime(state.NextRestartAt)); err != nil {
			return err
		}
	}

	if snapshot == nil {
		_, err := fmt.Fprintln(writer, i18n.T("诊断快照：不可用（尚未捕获故障快照）"))
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s: %s\n", i18n.T("快照捕获时间"), formatDiagnosticTime(snapshot.GeneratedAt)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s: %s\n", i18n.T("默认接口"), snapshot.DefaultInterface); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s: %s\n", i18n.T("bootstrap 目标"), strings.Join(snapshot.BootstrapTargets, ", ")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, i18n.T("探针：")); err != nil {
		return err
	}
	for _, probe := range snapshot.ProbeResults {
		result := i18n.T("正常")
		if !probe.OK {
			result = i18n.T("失败")
			if probe.Detail != "" {
				result += " (" + probe.Detail + ")"
			}
		}
		if _, err := fmt.Fprintf(writer, "  %s: %s\n", probe.Layer, result); err != nil {
			return err
		}
	}
	if len(snapshot.Sections) > 0 {
		if _, err := fmt.Fprintln(writer, i18n.T("诊断详情：")); err != nil {
			return err
		}
		keys := make([]string, 0, len(snapshot.Sections))
		for key := range snapshot.Sections {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if _, err := fmt.Fprintf(writer, "[%s]\n%s\n", key, snapshot.Sections[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatDiagnosticTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

// diagnosticFailureName is a presentation contract, kept intentionally
// shorter than watchdog policy reasons so CLI output stays stable if the
// policy's internal enum wording changes.
func diagnosticFailureName(failure sysd.FailureClass) string {
	switch failure {
	case sysd.FailurePhysicalUplink:
		return "physical_uplink"
	case sysd.FailureBootstrapDNS:
		return "bootstrap_dns"
	case sysd.FailureNodeResolution:
		return "node_resolution"
	case sysd.FailureMixedPort:
		return "mixed_port"
	case sysd.FailureEndToEnd:
		return "end_to_end"
	default:
		return string(failure)
	}
}
