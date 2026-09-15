package bootstrapdns

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestForwarderRejectsMalformedQueriesBeforeExchange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query []byte
	}{
		{name: "shorter than DNS header", query: make([]byte, 11)},
		{name: "query has response bit", query: dnsResponse(dnsQuery(0x1234))},
		{name: "query has no questions", query: dnsHeaderOnly(0x1234)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var exchanges atomic.Int32
			forwarder := newTestForwarder(t, func(context.Context, Upstream, []byte) ([]byte, error) {
				exchanges.Add(1)
				return nil, errors.New("must not exchange malformed input")
			})

			if _, err := forwarder.Resolve(context.Background(), tt.query); !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("Resolve() error = %v, want ErrInvalidQuery", err)
			}
			if got := exchanges.Load(); got != 0 {
				t.Fatalf("exchange called %d times for malformed query, want 0", got)
			}
		})
	}
}

func TestForwarderRejectsAnUnboundedUpstreamList(t *testing.T) {
	t.Parallel()
	upstreams := make([]Upstream, maxUpstreamCount+1)
	for index := range upstreams {
		upstreams[index] = Upstream{Name: "resolver", Network: "udp", Address: "192.0.2.1:53"}
	}
	_, err := NewForwarder(Config{
		Upstreams: upstreams, AttemptTimeout: time.Second, FailureCooldown: time.Second,
		MaxRequestBytes: 4096, MaxConcurrent: 1,
		Exchange: func(context.Context, Upstream, []byte) ([]byte, error) { return nil, errors.New("unused") },
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewForwarder() error = %v, want ErrInvalidConfig", err)
	}
}

func TestForwarderValidatesResponsesAndFallsBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response func([]byte) []byte
	}{
		{
			name: "short response",
			response: func([]byte) []byte {
				return make([]byte, 11)
			},
		},
		{
			name: "response bit is missing",
			response: func(query []byte) []byte {
				return append([]byte(nil), query...)
			},
		},
		{
			name: "transaction ID differs",
			response: func(query []byte) []byte {
				response := dnsResponse(query)
				response[1]++
				return response
			},
		},
		{
			name: "truncated response",
			response: func(query []byte) []byte {
				response := dnsResponse(query)
				response[2] |= 0x02
				return response
			},
		},
		{
			name: "server failure response",
			response: func(query []byte) []byte {
				response := dnsResponse(query)
				response[3] = 2
				return response
			},
		},
		{
			name: "refused response",
			response: func(query []byte) []byte {
				response := dnsResponse(query)
				response[3] = 5
				return response
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query := dnsQuery(0x1357)
			want := dnsResponse(query)
			var attempts []string
			forwarder := newTestForwarder(t, func(_ context.Context, upstream Upstream, request []byte) ([]byte, error) {
				attempts = append(attempts, upstream.Name)
				if upstream.Name == "primary" {
					return tt.response(request), nil
				}
				return dnsResponse(request), nil
			})

			got, err := forwarder.Resolve(context.Background(), query)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Resolve() = %v, want %v", got, want)
			}
			if wantAttempts := []string{"primary", "secondary"}; !reflect.DeepEqual(attempts, wantAttempts) {
				t.Fatalf("attempts = %v, want %v", attempts, wantAttempts)
			}
		})
	}
}

func TestForwarderReturnsNXDOMAINWithoutTryingAnotherUpstream(t *testing.T) {
	t.Parallel()

	query := dnsQuery(0x7788)
	var attempts []string
	forwarder := newTestForwarder(t, func(_ context.Context, upstream Upstream, request []byte) ([]byte, error) {
		attempts = append(attempts, upstream.Name)
		response := dnsResponse(request)
		response[3] = 3
		return response, nil
	})

	response, err := forwarder.Resolve(context.Background(), query)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if response[3]&0x0f != 3 {
		t.Fatalf("response rcode = %d, want NXDOMAIN", response[3]&0x0f)
	}
	if want := []string{"primary"}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
}

func TestForwarderFallsBackAfterPrimaryReset(t *testing.T) {
	t.Parallel()

	resetErr := errors.New("connection reset by peer")
	query := dnsQuery(0x2468)
	var attempts []string
	forwarder := newTestForwarder(t, func(_ context.Context, upstream Upstream, request []byte) ([]byte, error) {
		attempts = append(attempts, upstream.Name)
		if upstream.Name == "primary" {
			return nil, resetErr
		}
		return dnsResponse(request), nil
	})

	response, err := forwarder.Resolve(context.Background(), query)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := dnsResponse(query); !reflect.DeepEqual(response, want) {
		t.Fatalf("Resolve() = %v, want %v", response, want)
	}
	if want := []string{"primary", "secondary"}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
}

func TestForwarderFallsBackWhenPrimaryConnectsButDoesNotRespond(t *testing.T) {
	t.Parallel()

	query := dnsQuery(0x369c)
	var attempts []string
	forwarder := newTestForwarderWithConfig(t, Config{
		Upstreams: []Upstream{
			{Name: "primary", Network: "tcp", Address: "192.0.2.1:53"},
			{Name: "secondary", Network: "udp", Address: "198.51.100.1:53"},
		},
		AttemptTimeout:  20 * time.Millisecond,
		FailureCooldown: time.Minute,
		MaxRequestBytes: 4096,
		MaxConcurrent:   4,
		Exchange: func(ctx context.Context, upstream Upstream, request []byte) ([]byte, error) {
			attempts = append(attempts, upstream.Name)
			if upstream.Name == "primary" {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return dnsResponse(request), nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := forwarder.Resolve(ctx, query)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := dnsResponse(query); !reflect.DeepEqual(response, want) {
		t.Fatalf("Resolve() = %v, want %v", response, want)
	}
	if want := []string{"primary", "secondary"}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
}

func TestForwarderReturnsAnAggregateWhenAllUpstreamsFail(t *testing.T) {
	t.Parallel()

	primaryErr := errors.New("primary reset")
	secondaryErr := errors.New("secondary timeout")
	forwarder := newTestForwarder(t, func(_ context.Context, upstream Upstream, _ []byte) ([]byte, error) {
		if upstream.Name == "primary" {
			return nil, primaryErr
		}
		return nil, secondaryErr
	})

	_, err := forwarder.Resolve(context.Background(), dnsQuery(0x1111))
	if err == nil {
		t.Fatal("Resolve() error = nil, want aggregate failure")
	}
	if !errors.Is(err, primaryErr) || !errors.Is(err, secondaryErr) {
		t.Fatalf("Resolve() error = %v, want errors.Is true for both upstream failures", err)
	}
	for _, upstreamName := range []string{"primary", "secondary"} {
		if !strings.Contains(err.Error(), upstreamName) {
			t.Errorf("Resolve() error = %q, want upstream name %q", err, upstreamName)
		}
	}
}

func TestForwarderCoolsFailedUpstreamAndRecoversItHalfOpen(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	var attempts []string
	primaryCalls := 0
	forwarder := newTestForwarderWithConfig(t, Config{
		Upstreams: []Upstream{
			{Name: "primary", Network: "tcp", Address: "192.0.2.1:53"},
			{Name: "secondary", Network: "udp", Address: "198.51.100.1:53"},
		},
		AttemptTimeout:  time.Second,
		FailureCooldown: time.Minute,
		MaxRequestBytes: 4096,
		MaxConcurrent:   4,
		Now:             func() time.Time { return now },
		Exchange: func(_ context.Context, upstream Upstream, request []byte) ([]byte, error) {
			attempts = append(attempts, upstream.Name)
			if upstream.Name == "primary" {
				primaryCalls++
				if primaryCalls == 1 {
					return nil, errors.New("primary unavailable")
				}
			}
			return dnsResponse(request), nil
		},
	})

	resolve := func(id uint16) {
		t.Helper()
		if _, err := forwarder.Resolve(context.Background(), dnsQuery(id)); err != nil {
			t.Fatalf("Resolve(%#x) error = %v", id, err)
		}
	}

	resolve(1) // Primary fails and enters cooldown; secondary succeeds.
	resolve(2) // Primary is skipped while cooling down.
	now = now.Add(time.Minute)
	resolve(3) // Primary is tried half-open and recovers.
	resolve(4) // Recovered primary remains the preferred upstream.

	want := []string{"primary", "secondary", "secondary", "primary", "primary"}
	if !reflect.DeepEqual(attempts, want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
}

func TestForwarderDoesNotLetAnAttemptMutateTheCallersQuery(t *testing.T) {
	t.Parallel()

	query := dnsQuery(0xabcd)
	wantQuery := append([]byte(nil), query...)
	forwarder := newTestForwarder(t, func(_ context.Context, upstream Upstream, request []byte) ([]byte, error) {
		if upstream.Name == "primary" {
			request[0] = 0
			request[1] = 0
			return nil, errors.New("primary reset")
		}
		if request[0] != 0xab || request[1] != 0xcd {
			return nil, errors.New("secondary received query mutated by primary")
		}
		return dnsResponse(request), nil
	})

	if _, err := forwarder.Resolve(context.Background(), query); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !reflect.DeepEqual(query, wantQuery) {
		t.Fatalf("caller query mutated: got %v, want %v", query, wantQuery)
	}
}

func TestForwarderRejectsOversizedRequests(t *testing.T) {
	t.Parallel()

	var exchanges atomic.Int32
	forwarder := newTestForwarderWithConfig(t, Config{
		Upstreams:       []Upstream{{Name: "primary", Network: "tcp", Address: "192.0.2.1:53"}},
		AttemptTimeout:  time.Second,
		FailureCooldown: time.Minute,
		MaxRequestBytes: 32,
		MaxConcurrent:   1,
		Exchange: func(context.Context, Upstream, []byte) ([]byte, error) {
			exchanges.Add(1)
			return nil, errors.New("must not exchange oversized input")
		},
	})

	query := append(dnsQuery(0x1234), make([]byte, 64)...)
	if _, err := forwarder.Resolve(context.Background(), query); !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("Resolve() error = %v, want ErrRequestTooLarge", err)
	}
	if got := exchanges.Load(); got != 0 {
		t.Fatalf("exchange called %d times for oversized query, want 0", got)
	}
}

func TestForwarderRejectsWorkBeyondConcurrencyLimit(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	forwarder := newTestForwarderWithConfig(t, Config{
		Upstreams:       []Upstream{{Name: "primary", Network: "tcp", Address: "192.0.2.1:53"}},
		AttemptTimeout:  time.Second,
		FailureCooldown: time.Minute,
		MaxRequestBytes: 4096,
		MaxConcurrent:   1,
		Exchange: func(_ context.Context, _ Upstream, request []byte) ([]byte, error) {
			close(started)
			<-release
			return dnsResponse(request), nil
		},
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := forwarder.Resolve(context.Background(), dnsQuery(1))
		firstDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first Resolve() did not enter exchange")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := forwarder.Resolve(context.Background(), dnsQuery(2))
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		if !errors.Is(err, ErrConcurrencyLimit) {
			t.Fatalf("second Resolve() error = %v, want ErrConcurrencyLimit", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("second Resolve() blocked instead of enforcing the concurrency limit")
	}

	unblock()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first Resolve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first Resolve() did not finish after exchange was released")
	}
}

func newTestForwarder(t *testing.T, exchange ExchangeFunc) *Forwarder {
	t.Helper()
	return newTestForwarderWithConfig(t, Config{
		Upstreams: []Upstream{
			{Name: "primary", Network: "tcp", Address: "192.0.2.1:53"},
			{Name: "secondary", Network: "udp", Address: "198.51.100.1:53"},
		},
		AttemptTimeout:  50 * time.Millisecond,
		FailureCooldown: time.Minute,
		MaxRequestBytes: 4096,
		MaxConcurrent:   4,
		Exchange:        exchange,
	})
}

func newTestForwarderWithConfig(t *testing.T, config Config) *Forwarder {
	t.Helper()
	forwarder, err := NewForwarder(config)
	if err != nil {
		t.Fatalf("NewForwarder() error = %v", err)
	}
	return forwarder
}

func dnsHeaderOnly(id uint16) []byte {
	return []byte{
		byte(id >> 8), byte(id),
		0x01, 0x00,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
	}
}

func dnsQuery(id uint16) []byte {
	message := dnsHeaderOnly(id)
	message[5] = 1
	return append(message,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm',
		0x00,
		0x00, 0x01,
		0x00, 0x01,
	)
}

func dnsResponse(query []byte) []byte {
	response := append([]byte(nil), query...)
	if len(response) >= 4 {
		response[2] |= 0x80
	}
	return response
}
