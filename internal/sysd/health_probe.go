package sysd

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/bootstrapdns"
	"github.com/Trilives/sboxkit/internal/clashapi"
	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

const bootstrapProbeDomain = "connectivitycheck.gstatic.com"

type healthProbeRuntime struct {
	interfaceName     string
	forwarder         *bootstrapdns.Forwarder
	nodeDomains       []string
	bootstrapDegraded bool
}

func productionHealthProbes(
	service, tunDev, proxyAddr, probeURL string,
	attempts int,
	timeout, gap time.Duration,
) ([]HealthProbe, *healthProbeRuntime) {
	runtime := &healthProbeRuntime{}
	cfg := config.NormalizeBootstrapDNS(config.Load(paths.Detect()))

	probes := []HealthProbe{
		{Layer: ProbePhysicalUplink, Run: func() error {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			interfaceName, err := bootstrapdns.DiscoverDefaultInterface(ctx, []string{tunDev})
			if err != nil {
				return fmt.Errorf("physical_route_unavailable: %w", err)
			}
			if err := requireInterfaceUp(interfaceName); err != nil {
				return err
			}
			runtime.interfaceName = interfaceName
			return nil
		}},
		{Layer: ProbeBootstrapDNS, Run: func() error {
			forwarder, degraded, err := probeBootstrapUpstreams(cfg.BootstrapDNSUpstreams, runtime.interfaceName, timeout)
			runtime.forwarder = forwarder
			runtime.bootstrapDegraded = degraded
			return err
		}},
		{Layer: ProbeNodeResolution, Run: func() error {
			// Read the live selector immediately before this layer. A subscription
			// can contain many broken or dormant nodes; only the currently selected
			// chain affects traffic and should influence watchdog classification.
			runtime.nodeDomains = runtimeActiveNodeDomains(service)
			for index, domain := range runtime.nodeDomains {
				response, err := resolveHealthDomain(runtime.forwarder, domain, timeout)
				if err != nil {
					return fmt.Errorf("node_%d_query_failed: %w", index+1, err)
				}
				if err := requireSuccessfulDNSResponse(response, true); err != nil {
					return fmt.Errorf("node_%d_response_invalid: %w", index+1, err)
				}
			}
			return nil
		}},
		{Layer: ProbeMixedPort, Run: func() error {
			conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
			if err != nil {
				return fmt.Errorf("mixed_connect_failed: %w", err)
			}
			return conn.Close()
		}},
		{Layer: ProbeEndToEnd, Run: func() error {
			if !proxyWorks(proxyAddr, probeURL, attempts, timeout, gap) {
				return errors.New("proxy_http_probe_failed")
			}
			return nil
		}},
	}
	return probes, runtime
}

func probeBootstrapUpstreams(
	upstreams []config.BootstrapDNSUpstream,
	interfaceName string,
	timeout time.Duration,
) (*bootstrapdns.Forwarder, bool, error) {
	if len(upstreams) == 0 {
		return nil, false, errors.New("bootstrap_config_invalid: no upstreams")
	}
	primary, err := newHealthDNSForwarder(upstreams[:1], interfaceName, timeout)
	if err != nil {
		return nil, false, fmt.Errorf("bootstrap_config_invalid: %w", err)
	}
	response, primaryErr := resolveHealthDomain(primary, bootstrapProbeDomain, timeout)
	if primaryErr == nil {
		primaryErr = requireSuccessfulDNSResponse(response, false)
	}
	if primaryErr == nil {
		forwarder, buildErr := newHealthDNSForwarder(upstreams, interfaceName, timeout)
		return forwarder, false, buildErr
	}
	if len(upstreams) == 1 {
		return nil, false, fmt.Errorf("bootstrap_query_failed: %w", primaryErr)
	}
	fallback, err := newHealthDNSForwarder(upstreams[1:], interfaceName, timeout)
	if err != nil {
		return nil, false, fmt.Errorf("bootstrap_config_invalid: %w", err)
	}
	response, fallbackErr := resolveHealthDomain(fallback, bootstrapProbeDomain, timeout)
	if fallbackErr == nil {
		fallbackErr = requireSuccessfulDNSResponse(response, false)
	}
	if fallbackErr != nil {
		return nil, false, fmt.Errorf("bootstrap_query_failed: primary: %v; fallback: %w", primaryErr, fallbackErr)
	}
	return fallback, true, nil
}

func annotateBootstrapDegraded(report HealthReport, degraded bool) HealthReport {
	if !degraded {
		return report
	}
	annotated := report
	annotated.Results = append([]ProbeResult(nil), report.Results...)
	for index := range annotated.Results {
		if annotated.Results[index].Layer == ProbeBootstrapDNS && annotated.Results[index].OK {
			annotated.Results[index].Detail = "bootstrap_dns_degraded: fallback upstream active"
			break
		}
	}
	return annotated
}

func requireInterfaceUp(interfaceName string) error {
	state, err := os.ReadFile(filepath.Join("/sys/class/net", interfaceName, "operstate"))
	if err != nil {
		return fmt.Errorf("physical_interface_state_unavailable: %w", err)
	}
	switch strings.TrimSpace(string(state)) {
	case "up", "unknown":
		return nil
	default:
		return fmt.Errorf("physical_interface_down: %s", strings.TrimSpace(string(state)))
	}
}

func newHealthDNSForwarder(upstreams []config.BootstrapDNSUpstream, interfaceName string, timeout time.Duration) (*bootstrapdns.Forwarder, error) {
	exchange, err := bootstrapdns.NewDirectExchange(bootstrapdns.DirectExchangeConfig{
		Interface:        interfaceName,
		DialTimeout:      timeout,
		MaxResponseBytes: 64 * 1024,
	})
	if err != nil {
		return nil, err
	}
	converted := make([]bootstrapdns.Upstream, 0, len(upstreams))
	for _, upstream := range upstreams {
		converted = append(converted, bootstrapdns.Upstream{
			Name:          upstream.Provider,
			Network:       upstream.Type,
			Address:       net.JoinHostPort(upstream.Server, strconv.Itoa(upstream.Port)),
			Path:          upstream.Path,
			TLSServerName: upstream.TLSServerName,
		})
	}
	return bootstrapdns.NewForwarder(bootstrapdns.Config{
		Upstreams:       converted,
		AttemptTimeout:  timeout,
		FailureCooldown: 30 * time.Second,
		MaxRequestBytes: 4096,
		MaxConcurrent:   4,
		Exchange:        exchange,
	})
}

func resolveHealthDomain(forwarder *bootstrapdns.Forwarder, domain string, timeout time.Duration) ([]byte, error) {
	if forwarder == nil {
		return nil, errors.New("bootstrap forwarder unavailable")
	}
	query, err := buildDNSQuery(domain)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return forwarder.Resolve(ctx, query)
}

func buildDNSQuery(domain string) ([]byte, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if domain == "" || len(domain) > 253 {
		return nil, errors.New("invalid DNS name")
	}
	message := make([]byte, 12, 12+len(domain)+6)
	if _, err := rand.Read(message[:2]); err != nil {
		return nil, fmt.Errorf("generate DNS transaction ID: %w", err)
	}
	message[2] = 0x01
	binary.BigEndian.PutUint16(message[4:6], 1)
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil, errors.New("invalid DNS label")
		}
		message = append(message, byte(len(label)))
		message = append(message, label...)
	}
	message = append(message, 0, 0, 1, 0, 1)
	return message, nil
}

func requireSuccessfulDNSResponse(response []byte, requireAnswer bool) error {
	if len(response) < 12 {
		return errors.New("short DNS response")
	}
	if response[3]&0x0f != 0 {
		return fmt.Errorf("DNS rcode %d", response[3]&0x0f)
	}
	if requireAnswer && binary.BigEndian.Uint16(response[6:8]) == 0 {
		return errors.New("DNS response has no answers")
	}
	return nil
}

type currentSelectorFunc func(string) (string, error)

type runtimeOutbound struct {
	typeName  string
	server    string
	defaultTo string
	outbounds []string
}

func runtimeActiveNodeDomains(service string) []string {
	raw, err := os.ReadFile(filepath.Join(paths.RuntimeDir, service+".json"))
	if err != nil {
		return nil
	}
	var document map[string]any
	if len(raw) > 1024*1024 || json.Unmarshal(raw, &document) != nil {
		return nil
	}
	var current currentSelectorFunc
	if client := clashapi.FromConfig(document); client != nil {
		current = client.Current
	}
	return activeNodeDomainsFromDocument(document, current)
}

func activeNodeDomainsFromDocument(document map[string]any, current currentSelectorFunc) []string {
	outbounds := make(map[string]runtimeOutbound)
	order := make([]string, 0)
	items, _ := document["outbounds"].([]any)
	for _, item := range items {
		fields, _ := item.(map[string]any)
		tag, _ := fields["tag"].(string)
		if tag == "" {
			continue
		}
		descriptor := runtimeOutbound{}
		descriptor.typeName, _ = fields["type"].(string)
		descriptor.server, _ = fields["server"].(string)
		descriptor.defaultTo, _ = fields["default"].(string)
		members, _ := fields["outbounds"].([]any)
		for _, member := range members {
			if value, ok := member.(string); ok && value != "" {
				descriptor.outbounds = append(descriptor.outbounds, value)
			}
		}
		outbounds[tag] = descriptor
		order = append(order, tag)
	}
	if len(outbounds) == 0 {
		return nil
	}

	start := "Proxy"
	if _, found := outbounds[start]; !found {
		if route, ok := document["route"].(map[string]any); ok {
			start, _ = route["final"].(string)
		}
	}
	if _, found := outbounds[start]; !found {
		start = ""
		for _, tag := range order {
			candidate := outbounds[tag]
			if candidate.typeName == "selector" || candidate.typeName == "urltest" {
				start = tag
				break
			}
		}
	}
	if start == "" {
		return nil
	}

	seen := make(map[string]struct{})
	for depth, tag := 0, start; depth < len(outbounds)+1; depth++ {
		if _, cycle := seen[tag]; cycle {
			return nil
		}
		seen[tag] = struct{}{}
		outbound, found := outbounds[tag]
		if !found {
			return nil
		}
		if outbound.typeName != "selector" && outbound.typeName != "urltest" {
			domain := strings.TrimSpace(outbound.server)
			if domain == "" || net.ParseIP(strings.Trim(domain, "[]")) != nil {
				return nil
			}
			return []string{domain}
		}

		next := ""
		if current != nil {
			var err error
			next, err = current(tag)
			if err != nil {
				// A configured live API is authoritative. Guessing a stale default
				// here can classify an unused node as broken and restart healthy
				// traffic; the end-to-end layer still detects a real outage.
				return nil
			}
		} else {
			next = outbound.defaultTo
			if next == "" && len(outbound.outbounds) > 0 {
				next = outbound.outbounds[0]
			}
		}
		if next == "" {
			return nil
		}
		tag = next
	}
	return nil
}

func bootstrapTargetLabels(upstreams []config.BootstrapDNSUpstream) []string {
	labels := make([]string, 0, len(upstreams))
	for _, upstream := range upstreams {
		labels = append(labels, upstream.Type+"://"+net.JoinHostPort(upstream.Server, strconv.Itoa(upstream.Port)))
	}
	return labels
}
