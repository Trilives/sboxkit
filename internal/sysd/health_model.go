package sysd

import (
	"fmt"
	"time"
)

// FailureClass is deliberately stable: these values are written to logs and
// diagnostic files which may outlive the sboxkit version that created them.
type FailureClass string

const (
	FailureNone           FailureClass = "healthy"
	FailurePhysicalUplink FailureClass = "physical_uplink_unavailable"
	FailureBootstrapDNS   FailureClass = "bootstrap_dns_unavailable"
	FailureNodeResolution FailureClass = "node_resolution_failed"
	FailureMixedPort      FailureClass = "mixed_port_unavailable"
	FailureEndToEnd       FailureClass = "end_to_end_failed"
)

// ProbeLayer identifies one dependency in the network path, ordered from the
// host network outwards to the full proxy path.
type ProbeLayer string

const (
	ProbePhysicalUplink ProbeLayer = "physical_uplink"
	ProbeBootstrapDNS   ProbeLayer = "bootstrap_dns"
	ProbeNodeResolution ProbeLayer = "node_resolution"
	ProbeMixedPort      ProbeLayer = "mixed_port"
	ProbeEndToEnd       ProbeLayer = "end_to_end"
)

type ProbeResult struct {
	Layer    ProbeLayer    `json:"layer"`
	OK       bool          `json:"ok"`
	Detail   string        `json:"detail,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
}

type HealthReport struct {
	Healthy bool          `json:"healthy"`
	Failure FailureClass  `json:"failure"`
	Results []ProbeResult `json:"results"`
}

type HealthProbe struct {
	Layer ProbeLayer
	Run   func() error
}

// RunLayeredProbes stops at the first broken dependency. Later checks would
// only add noise and can accidentally re-enter the failed proxy/DNS path.
func RunLayeredProbes(probes []HealthProbe) HealthReport {
	results := make([]ProbeResult, 0, len(probes))
	for _, probe := range probes {
		started := time.Now()
		err := runHealthProbe(probe)
		result := ProbeResult{Layer: probe.Layer, OK: err == nil, Duration: time.Since(started)}
		if err != nil {
			result.Detail = err.Error()
			results = append(results, result)
			return HealthReport{Failure: failureForLayer(probe.Layer), Results: results}
		}
		results = append(results, result)
	}
	return HealthReport{Healthy: true, Failure: FailureNone, Results: results}
}

func runHealthProbe(probe HealthProbe) (err error) {
	if probe.Run == nil {
		return fmt.Errorf("probe %s has no implementation", probe.Layer)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("probe %s panicked: %v", probe.Layer, recovered)
		}
	}()
	return probe.Run()
}

func failureForLayer(layer ProbeLayer) FailureClass {
	switch layer {
	case ProbePhysicalUplink:
		return FailurePhysicalUplink
	case ProbeBootstrapDNS:
		return FailureBootstrapDNS
	case ProbeNodeResolution:
		return FailureNodeResolution
	case ProbeMixedPort:
		return FailureMixedPort
	case ProbeEndToEnd:
		return FailureEndToEnd
	default:
		return FailureEndToEnd
	}
}
