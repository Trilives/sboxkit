package bootstrapdns

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidConfig    = errors.New("invalid bootstrap DNS configuration")
	ErrInvalidQuery     = errors.New("invalid DNS query")
	ErrInvalidResponse  = errors.New("invalid DNS response")
	ErrRequestTooLarge  = errors.New("DNS request is too large")
	ErrResponseTooLarge = errors.New("DNS response is too large")
	ErrConcurrencyLimit = errors.New("bootstrap DNS concurrency limit reached")
	ErrNoUpstream       = errors.New("no bootstrap DNS upstream is available")
)

const maxUpstreamCount = 8

// Upstream is a direct, IP-literal DNS endpoint. Address must include its port.
// HTTPS uses Path for the DoH resource and TLSServerName for certificate checks.
type Upstream struct {
	Name          string `json:"name,omitempty"`
	Network       string `json:"network"`
	Address       string `json:"address"`
	Path          string `json:"path,omitempty"`
	TLSServerName string `json:"tls_server_name,omitempty"`
}

// ExchangeFunc performs exactly one direct upstream exchange.
type ExchangeFunc func(context.Context, Upstream, []byte) ([]byte, error)

// Config controls ordered fallback and resource bounds.
type Config struct {
	Upstreams       []Upstream
	AttemptTimeout  time.Duration
	FailureCooldown time.Duration
	MaxRequestBytes int
	MaxConcurrent   int
	Exchange        ExchangeFunc
	Now             func() time.Time
}

type upstreamState struct {
	failedUntil time.Time
	probing     bool
}

// Forwarder safely forwards validated DNS queries through an ordered list.
type Forwarder struct {
	upstreams       []Upstream
	attemptTimeout  time.Duration
	failureCooldown time.Duration
	maxRequestBytes int
	exchange        ExchangeFunc
	now             func() time.Time
	sem             chan struct{}

	mu     sync.Mutex
	states []upstreamState
}

func NewForwarder(config Config) (*Forwarder, error) {
	if len(config.Upstreams) == 0 || len(config.Upstreams) > maxUpstreamCount || config.AttemptTimeout <= 0 || config.FailureCooldown <= 0 ||
		config.MaxRequestBytes < 12 || config.MaxConcurrent <= 0 || config.Exchange == nil {
		return nil, ErrInvalidConfig
	}
	upstreams := append([]Upstream(nil), config.Upstreams...)
	for index := range upstreams {
		normalized, err := normalizeUpstream(upstreams[index], index)
		if err != nil {
			return nil, err
		}
		upstreams[index] = normalized
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Forwarder{
		upstreams:       upstreams,
		attemptTimeout:  config.AttemptTimeout,
		failureCooldown: config.FailureCooldown,
		maxRequestBytes: config.MaxRequestBytes,
		exchange:        config.Exchange,
		now:             now,
		sem:             make(chan struct{}, config.MaxConcurrent),
		states:          make([]upstreamState, len(upstreams)),
	}, nil
}

func normalizeUpstream(upstream Upstream, index int) (Upstream, error) {
	normalized := upstream
	normalized.Name = strings.TrimSpace(normalized.Name)
	if normalized.Name == "" {
		normalized.Name = fmt.Sprintf("upstream-%d", index+1)
	}
	if len(normalized.Name) > 64 || strings.IndexFunc(normalized.Name, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return Upstream{}, fmt.Errorf("%w: upstream name contains control characters or exceeds 64 bytes", ErrInvalidConfig)
	}
	normalized.Network = strings.ToLower(strings.TrimSpace(normalized.Network))
	normalized.Address = strings.TrimSpace(normalized.Address)
	normalized.Path = strings.TrimSpace(normalized.Path)
	normalized.TLSServerName = strings.TrimSpace(normalized.TLSServerName)
	if normalized.Network != "udp" && normalized.Network != "tcp" && normalized.Network != "https" {
		return Upstream{}, fmt.Errorf("%w: %s uses unsupported transport %q", ErrInvalidConfig, normalized.Name, normalized.Network)
	}
	host, portText, err := net.SplitHostPort(normalized.Address)
	port, portErr := strconv.Atoi(portText)
	if err != nil || net.ParseIP(strings.Trim(host, "[]")) == nil || portErr != nil || port < 1 || port > 65535 {
		return Upstream{}, fmt.Errorf("%w: %s address must be an IP literal with a port", ErrInvalidConfig, normalized.Name)
	}
	if normalized.Network == "https" {
		if normalized.Path == "" {
			normalized.Path = "/dns-query"
		}
		parsedPath, parseErr := url.ParseRequestURI(normalized.Path)
		if parseErr != nil || parsedPath.IsAbs() || !strings.HasPrefix(normalized.Path, "/") {
			return Upstream{}, fmt.Errorf("%w: %s has an invalid HTTPS path", ErrInvalidConfig, normalized.Name)
		}
		if normalized.TLSServerName == "" {
			return Upstream{}, fmt.Errorf("%w: %s requires a TLS server name", ErrInvalidConfig, normalized.Name)
		}
	}
	return normalized, nil
}

func (f *Forwarder) Resolve(ctx context.Context, query []byte) ([]byte, error) {
	if len(query) > f.maxRequestBytes {
		return nil, ErrRequestTooLarge
	}
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	default:
		return nil, ErrConcurrencyLimit
	}

	var failures []error
	for index, upstream := range f.upstreams {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		if !f.claim(index) {
			continue
		}
		request := append([]byte(nil), query...)
		attemptCtx, cancel := context.WithTimeout(ctx, f.attemptTimeout)
		response, err := f.exchange(attemptCtx, upstream, request)
		cancel()
		if err == nil {
			err = validateResponse(query, response)
		}
		if err == nil {
			f.recordSuccess(index)
			return append([]byte(nil), response...), nil
		}
		f.recordFailure(index)
		failures = append(failures, fmt.Errorf("bootstrap DNS upstream %s: %w", upstream.Name, err))
	}
	if len(failures) == 0 {
		return nil, ErrNoUpstream
	}
	return nil, errors.Join(failures...)
}

func (f *Forwarder) claim(index int) bool {
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	state := &f.states[index]
	if state.probing || now.Before(state.failedUntil) {
		return false
	}
	// A non-zero expired deadline is a half-open probe. Permit only one.
	if !state.failedUntil.IsZero() {
		state.probing = true
	}
	return true
}

func (f *Forwarder) recordFailure(index int) {
	f.mu.Lock()
	f.states[index] = upstreamState{failedUntil: f.now().Add(f.failureCooldown)}
	f.mu.Unlock()
}

func (f *Forwarder) recordSuccess(index int) {
	f.mu.Lock()
	f.states[index] = upstreamState{}
	f.mu.Unlock()
}

func validateQuery(message []byte) error {
	if len(message) < 12 || message[2]&0x80 != 0 || binary.BigEndian.Uint16(message[4:6]) == 0 {
		return ErrInvalidQuery
	}
	offset := 12
	questions := int(binary.BigEndian.Uint16(message[4:6]))
	for range questions {
		next, err := skipDNSName(message, offset)
		if err != nil || next+4 > len(message) {
			return ErrInvalidQuery
		}
		offset = next + 4
	}
	return nil
}

func skipDNSName(message []byte, offset int) (int, error) {
	labels := 0
	for {
		if offset >= len(message) || labels > 127 {
			return 0, ErrInvalidQuery
		}
		length := int(message[offset])
		offset++
		switch {
		case length == 0:
			return offset, nil
		case length&0xc0 == 0xc0:
			if offset >= len(message) {
				return 0, ErrInvalidQuery
			}
			return offset + 1, nil
		case length > 63 || offset+length > len(message):
			return 0, ErrInvalidQuery
		default:
			offset += length
			labels++
		}
	}
}

func validateResponse(query, response []byte) error {
	if len(response) < 12 || response[2]&0x80 == 0 || response[0] != query[0] || response[1] != query[1] {
		return ErrInvalidResponse
	}
	if response[2]&0x02 != 0 {
		return fmt.Errorf("%w: response is truncated", ErrInvalidResponse)
	}
	rcode := response[3] & 0x0f
	// NXDOMAIN is a complete, authoritative answer for the queried name. Other
	// error rcodes describe an upstream failure and must advance the ordered
	// fallback chain instead of poisoning callers with a transient failure.
	if rcode != 0 && rcode != 3 {
		return fmt.Errorf("%w: upstream returned rcode %d", ErrInvalidResponse, rcode)
	}
	return nil
}
