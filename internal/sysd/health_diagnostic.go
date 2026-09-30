package sysd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const defaultDiagnosticMaxBytes = 64 * 1024

const (
	diagnosticCommandTimeout = 2 * time.Second
	diagnosticSectionBytes   = 8 * 1024
)

type DiagnosticSnapshot struct {
	GeneratedAt      time.Time         `json:"generated_at"`
	Failure          FailureClass      `json:"failure"`
	DefaultInterface string            `json:"default_interface,omitempty"`
	BootstrapTargets []string          `json:"bootstrap_targets,omitempty"`
	ProbeResults     []ProbeResult     `json:"probe_results,omitempty"`
	Sections         map[string]string `json:"sections,omitempty"`
	Truncated        bool              `json:"truncated,omitempty"`
}

var diagnosticRedactors = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*(?:bearer|basic)\s+)[^\s"']+`),
	regexp.MustCompile(`(?i)((?:password|passwd|token|api[_-]?key|secret)\s*[:=]\s*)[^\s&"']+`),
	regexp.MustCompile(`(?i)([?&](?:password|passwd|token|api[_-]?key|secret)=)[^&\s"']+`),
	regexp.MustCompile(`(?i)(://[^:/@\s]+:)[^@/\s]+(@)`),
}

// WriteDiagnosticSnapshotOnce preserves the first evidence in an incident.
// A separate flock file makes the existence check and atomic rename safe
// across concurrent timer invocations.
func WriteDiagnosticSnapshotOnce(path string, snapshot DiagnosticSnapshot, maxBytes int) (bool, error) {
	if maxBytes <= 0 {
		maxBytes = defaultDiagnosticMaxBytes
	}
	created := false
	err := withHealthFileLock(path, func() error {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect diagnostic snapshot: %w", err)
		}
		raw, err := marshalBoundedSnapshot(redactSnapshot(snapshot), maxBytes)
		if err != nil {
			return err
		}
		// Snapshot content is redacted before this point and must remain readable
		// through the non-privileged diagnostic command while the failure is live.
		if err := atomicWriteFile(path, raw, 0o644); err != nil {
			return fmt.Errorf("write diagnostic snapshot: %w", err)
		}
		created = true
		return nil
	})
	return created, err
}

// RemoveDiagnosticSnapshot coordinates recovery cleanup with snapshot writers.
func RemoveDiagnosticSnapshot(path string) error {
	return withHealthFileLock(path, func() error {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

func redactSnapshot(snapshot DiagnosticSnapshot) DiagnosticSnapshot {
	redacted := snapshot
	redacted.BootstrapTargets = append([]string(nil), snapshot.BootstrapTargets...)
	redacted.ProbeResults = append([]ProbeResult(nil), snapshot.ProbeResults...)
	for index := range redacted.ProbeResults {
		redacted.ProbeResults[index].Detail = redactDiagnosticText(redacted.ProbeResults[index].Detail)
	}
	redacted.Sections = make(map[string]string, len(snapshot.Sections))
	for key, value := range snapshot.Sections {
		redacted.Sections[key] = redactDiagnosticText(value)
	}
	return redacted
}

func redactDiagnosticText(value string) string {
	for index, pattern := range diagnosticRedactors {
		switch index {
		case 3:
			value = pattern.ReplaceAllString(value, `${1}[REDACTED]${2}`)
		default:
			value = pattern.ReplaceAllString(value, `${1}[REDACTED]`)
		}
	}
	return value
}

func marshalBoundedSnapshot(snapshot DiagnosticSnapshot, maxBytes int) ([]byte, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode diagnostic snapshot: %w", err)
	}
	if len(raw) <= maxBytes {
		return raw, nil
	}
	snapshot.Truncated = true

	// Reduce the largest free-text value first. This retains the structural
	// evidence and makes progress predictable even with multi-byte UTF-8.
	for {
		raw, err = json.Marshal(snapshot)
		if err != nil {
			return nil, fmt.Errorf("encode bounded diagnostic snapshot: %w", err)
		}
		if len(raw) <= maxBytes {
			return raw, nil
		}
		kind, key, index, value := largestSnapshotText(snapshot)
		if value == "" {
			return nil, fmt.Errorf("diagnostic byte limit %d is too small for required metadata", maxBytes)
		}
		shortened := truncateUTF8(value, len([]byte(value))/2)
		if shortened == value {
			shortened = ""
		}
		switch kind {
		case "section":
			snapshot.Sections[key] = shortened
		case "detail":
			snapshot.ProbeResults[index].Detail = shortened
		case "target":
			snapshot.BootstrapTargets[index] = shortened
		case "interface":
			snapshot.DefaultInterface = shortened
		}
	}
}

func largestSnapshotText(snapshot DiagnosticSnapshot) (kind, key string, index int, value string) {
	bestLength := len([]byte(snapshot.DefaultInterface))
	kind, index, value = "interface", -1, snapshot.DefaultInterface
	keys := make([]string, 0, len(snapshot.Sections))
	for section := range snapshot.Sections {
		keys = append(keys, section)
	}
	sort.Strings(keys)
	for _, section := range keys {
		if length := len([]byte(snapshot.Sections[section])); length > bestLength {
			bestLength, kind, key, index, value = length, "section", section, -1, snapshot.Sections[section]
		}
	}
	for probeIndex, result := range snapshot.ProbeResults {
		if length := len([]byte(result.Detail)); length > bestLength {
			bestLength, kind, key, index, value = length, "detail", "", probeIndex, result.Detail
		}
	}
	for targetIndex, target := range snapshot.BootstrapTargets {
		if length := len([]byte(target)); length > bestLength {
			bestLength, kind, key, index, value = length, "target", "", targetIndex, target
		}
	}
	return kind, key, index, value
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return strings.TrimSpace(value[:end])
}

func collectDiagnosticSnapshot(
	report HealthReport,
	interfaceName, tunDev, service string,
	bootstrapTargets []string,
) DiagnosticSnapshot {
	return collectDiagnosticSnapshotWithRunner(report, interfaceName, tunDev, service, bootstrapTargets, diagnosticCommand)
}

type diagnosticRunner func(string, ...string) string

type diagnosticSpec struct {
	key  string
	name string
	args []string
}

func collectDiagnosticSnapshotWithRunner(
	report HealthReport,
	interfaceName, tunDev, service string,
	bootstrapTargets []string,
	run diagnosticRunner,
) DiagnosticSnapshot {
	specs := []diagnosticSpec{
		{key: "ip_rule", name: "ip", args: []string{"-json", "rule", "show"}},
		{key: "routes", name: "ip", args: []string{"-json", "route", "show", "table", "all"}},
		{key: "service_lifecycle", name: "systemctl", args: serviceShowArgs(currentScope(), service)},
		{key: "interface_lifecycle", name: "ip", args: []string{"-json", "-details", "link", "show", "dev", interfaceName}},
		{key: "tun_lifecycle", name: "ip", args: []string{"-json", "-details", "link", "show", "dev", tunDev}},
		{key: "sockets", name: "ss", args: []string{"-H", "-s"}},
		{key: "conntrack", name: "conntrack", args: []string{"-S"}},
	}
	sections := make(map[string]string, len(specs)+1)
	var mutex sync.Mutex
	var group sync.WaitGroup
	for _, spec := range specs {
		spec := spec
		group.Add(1)
		go func() {
			defer group.Done()
			value := run(spec.name, spec.args...)
			mutex.Lock()
			sections[spec.key] = value
			mutex.Unlock()
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		value := resolverDiagnosticWithRunner(run)
		mutex.Lock()
		sections["resolver"] = value
		mutex.Unlock()
	}()
	group.Wait()
	return DiagnosticSnapshot{
		GeneratedAt:      time.Now().UTC(),
		Failure:          report.Failure,
		DefaultInterface: interfaceName,
		BootstrapTargets: append([]string(nil), bootstrapTargets...),
		ProbeResults:     append([]ProbeResult(nil), report.Results...),
		Sections:         sections,
	}
}

func diagnosticCommand(name string, args ...string) string {
	if strings.TrimSpace(name) == "" {
		return "unavailable: empty command"
	}
	ctx, cancel := context.WithTimeout(context.Background(), diagnosticCommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	reader, writer := io.Pipe()
	command.Stdout = writer
	command.Stderr = writer
	if err := command.Start(); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return "unavailable: " + err.Error()
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
		_ = writer.Close()
	}()
	raw, _ := io.ReadAll(io.LimitReader(reader, diagnosticSectionBytes+1))
	_ = reader.Close()
	err := <-done
	if len(raw) > diagnosticSectionBytes {
		raw = raw[:diagnosticSectionBytes]
	}
	text := strings.TrimSpace(string(raw))
	if ctx.Err() != nil {
		return truncateUTF8(text+"\n[command timed out]", diagnosticSectionBytes)
	}
	if err != nil {
		if text == "" {
			return "unavailable: " + err.Error()
		}
		text += "\n[command failed: " + err.Error() + "]"
	}
	return truncateUTF8(text, diagnosticSectionBytes)
}

func resolverDiagnostic() string {
	return resolverDiagnosticWithRunner(diagnosticCommand)
}

func resolverDiagnosticWithRunner(run diagnosticRunner) string {
	if value := run("resolvectl", "status"); !strings.HasPrefix(value, "unavailable:") {
		return value
	}
	raw, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return "unavailable: " + err.Error()
	}
	return truncateUTF8(string(raw), diagnosticSectionBytes)
}

func serviceShowArgs(s unitScope, service string) []string {
	return s.systemctl("show", service, "--property=ActiveState,SubState,ActiveEnterTimestamp,ExecMainStartTimestamp,NRestarts")[1:]
}
