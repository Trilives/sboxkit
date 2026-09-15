package bootstrapdns

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadRuntimeConfigStrictlyDecodesUpstreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "valid", content: `{"upstreams":[{"provider":"primary","type":"tcp","server":"223.5.5.5","port":53}]}`},
		{name: "unknown field", content: `{"upstreams":[],"secret":"no"}`, wantErr: true},
		{name: "trailing value", content: `{"upstreams":[]} {}`, wantErr: true},
		{name: "empty upstreams", content: `{"upstreams":[]}`, wantErr: true},
		{name: "malformed", content: `{`, wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "bootstrap.json")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			config, err := LoadRuntimeConfig(path)
			if (err != nil) != test.wantErr {
				t.Fatalf("LoadRuntimeConfig() error = %v, wantErr %v", err, test.wantErr)
			}
			if !test.wantErr && len(config.Upstreams) != 1 {
				t.Fatalf("upstream count = %d, want 1", len(config.Upstreams))
			}
		})
	}
}

func TestRuntimeConfigBuildsAnImmutableForwarderConfig(t *testing.T) {
	t.Parallel()

	runtimeConfig := RuntimeConfig{Upstreams: []RuntimeUpstream{
		{Provider: "primary", Type: "tcp", Server: "223.5.5.5", Port: 53},
		{Provider: "fallback", Type: "https", Server: "1.1.1.1", Port: 443, Path: "/dns-query", TLSServerName: "cloudflare-dns.com"},
	}}
	exchange := func(context.Context, Upstream, []byte) ([]byte, error) { return nil, errors.New("unused") }
	config, err := runtimeConfig.ForwarderConfig(exchange)
	if err != nil {
		t.Fatal(err)
	}
	runtimeConfig.Upstreams[0].Server = "192.0.2.1"
	if got := config.Upstreams[0].Address; got != "223.5.5.5:53" {
		t.Fatalf("forwarder config changed with caller input: %q", got)
	}
	if got := config.Upstreams[1]; got.Network != "https" || got.TLSServerName != "cloudflare-dns.com" {
		t.Fatalf("HTTPS upstream = %#v", got)
	}
}

func TestRuntimeConfigRejectsNonLiteralOrInvalidEndpoints(t *testing.T) {
	t.Parallel()

	for _, upstream := range []RuntimeUpstream{
		{Provider: "domain", Type: "tcp", Server: "dns.example", Port: 53},
		{Provider: "zero", Type: "tcp", Server: "192.0.2.1", Port: 0},
		{Provider: "high", Type: "tcp", Server: "192.0.2.1", Port: 65536},
	} {
		if _, err := (RuntimeConfig{Upstreams: []RuntimeUpstream{upstream}}).ForwarderConfig(DirectExchange); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("ForwarderConfig(%#v) error = %v, want ErrInvalidConfig", upstream, err)
		}
	}
}

func TestRunContextWithDepsWiresValidatedConfigWithoutSockets(t *testing.T) {
	t.Parallel()

	var calls []string
	var gotServer ServerConfig
	var gotExcluded []string
	deps := RunDeps{
		LoadRuntimeConfig: func(path string) (RuntimeConfig, error) {
			calls = append(calls, "load:"+path)
			return RuntimeConfig{Upstreams: []RuntimeUpstream{{Provider: "one", Type: "udp", Server: "192.0.2.1", Port: 53}}}, nil
		},
		NewDirectExchange: func(config DirectExchangeConfig) (ExchangeFunc, error) {
			calls = append(calls, "exchange")
			gotExcluded = append([]string(nil), config.ExcludedInterfaces...)
			return func(_ context.Context, _ Upstream, query []byte) ([]byte, error) { return dnsResponse(query), nil }, nil
		},
		Serve: func(_ context.Context, config ServerConfig, forwarder *Forwarder) error {
			calls = append(calls, "serve")
			gotServer = config
			if forwarder == nil {
				t.Fatal("Serve received nil forwarder")
			}
			return nil
		},
		DiscoverInterface: func(context.Context, []string) (string, error) { return "eth0", nil },
	}
	var stderr bytes.Buffer
	err := RunContextWithDeps(context.Background(), []string{
		"--config", "/runtime/bootstrap.json", "--listen", "127.0.0.1:6451", "--tun-dev", "tun-test",
	}, &stderr, deps)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"load:/runtime/bootstrap.json", "exchange", "serve"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("call order = %v, want %v", calls, want)
	}
	if gotServer.ListenAddress != "127.0.0.1:6451" {
		t.Fatalf("server config = %#v", gotServer)
	}
	if !reflect.DeepEqual(gotExcluded, []string{"tun-test"}) {
		t.Fatalf("excluded interfaces = %v", gotExcluded)
	}
}

func TestRunContextWithDepsRejectsUnsafeOrIncompleteInvocation(t *testing.T) {
	t.Parallel()

	complete := RunDeps{
		LoadRuntimeConfig: func(string) (RuntimeConfig, error) { return RuntimeConfig{}, errors.New("stop") },
		NewDirectExchange: func(DirectExchangeConfig) (ExchangeFunc, error) { return nil, errors.New("stop") },
		Serve:             func(context.Context, ServerConfig, *Forwarder) error { return errors.New("stop") },
		DiscoverInterface: func(context.Context, []string) (string, error) { return "eth0", nil },
	}
	for _, args := range [][]string{
		nil,
		{"--config", "x", "extra"},
		{"--config", "x", "--listen", "0.0.0.0:53"},
	} {
		if err := RunContextWithDeps(context.Background(), args, &bytes.Buffer{}, complete); err == nil {
			t.Errorf("RunContextWithDeps(%v) error = nil", args)
		}
	}
	if err := RunContextWithDeps(context.Background(), []string{"--config", "x"}, &bytes.Buffer{}, RunDeps{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("incomplete deps error = %v, want ErrInvalidConfig", err)
	}
}

func TestRunContextUsesDefaultDependenciesBeforeRejectingMissingConfig(t *testing.T) {
	t.Parallel()
	if err := RunContext(context.Background(), nil, &bytes.Buffer{}); err == nil {
		t.Fatal("RunContext() error = nil")
	}
}

func TestDefaultRunDependenciesAreComplete(t *testing.T) {
	t.Parallel()
	deps := defaultRunDeps()
	if deps.LoadRuntimeConfig == nil || deps.NewDirectExchange == nil || deps.Serve == nil || deps.DiscoverInterface == nil {
		t.Fatalf("default dependencies are incomplete: %#v", deps)
	}
}

func TestRunReturnsUsageFailureWithoutConfig(t *testing.T) {
	if code := Run(nil); code != 1 {
		t.Fatalf("Run(nil) = %d, want 1", code)
	}
}

func TestListenUDPUsesValidatedLoopbackAddress(t *testing.T) {
	t.Parallel()

	var gotNetwork, gotAddress string
	want := &stubPacketConn{}
	got, err := ListenUDP(ServerConfig{ListenAddress: "127.0.0.1:5353"}, func(network, address string) (net.PacketConn, error) {
		gotNetwork, gotAddress = network, address
		return want, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want || gotNetwork != "udp" || gotAddress != "127.0.0.1:5353" {
		t.Fatalf("ListenUDP() = (%T, %q, %q)", got, gotNetwork, gotAddress)
	}
}

type stubPacketConn struct{}

func (*stubPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, errors.New("not implemented")
}
func (*stubPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	return 0, errors.New("not implemented")
}
func (*stubPacketConn) Close() error                     { return nil }
func (*stubPacketConn) LocalAddr() net.Addr              { return stubAddr("127.0.0.1:5353") }
func (*stubPacketConn) SetDeadline(time.Time) error      { return nil }
func (*stubPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (*stubPacketConn) SetWriteDeadline(time.Time) error { return nil }

var _ net.PacketConn = (*stubPacketConn)(nil)

func TestLoadRuntimeConfigMissingFileExplainsOperation(t *testing.T) {
	t.Parallel()

	_, err := LoadRuntimeConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "open bootstrap DNS runtime config") {
		t.Fatalf("error = %v", err)
	}
}
