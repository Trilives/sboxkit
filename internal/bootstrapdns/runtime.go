package bootstrapdns

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxRuntimeConfigBytes = 1024 * 1024

type RuntimeUpstream struct {
	Provider      string `json:"provider"`
	Type          string `json:"type"`
	Server        string `json:"server"`
	Port          int    `json:"port"`
	Path          string `json:"path,omitempty"`
	TLSServerName string `json:"tls_server_name,omitempty"`
}

type RuntimeConfig struct {
	Upstreams []RuntimeUpstream `json:"upstreams"`
}

func LoadRuntimeConfig(path string) (RuntimeConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("open bootstrap DNS runtime config: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxRuntimeConfigBytes+1))
	decoder.DisallowUnknownFields()
	var config RuntimeConfig
	if err := decoder.Decode(&config); err != nil {
		return RuntimeConfig{}, fmt.Errorf("decode bootstrap DNS runtime config: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return RuntimeConfig{}, err
	}
	if len(config.Upstreams) == 0 {
		return RuntimeConfig{}, fmt.Errorf("%w: runtime config has no upstreams", ErrInvalidConfig)
	}
	return config, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode bootstrap DNS runtime config: trailing JSON value")
		}
		return fmt.Errorf("decode bootstrap DNS runtime config: %w", err)
	}
	return nil
}

func (config RuntimeConfig) ForwarderConfig(exchange ExchangeFunc) (Config, error) {
	upstreams := make([]Upstream, 0, len(config.Upstreams))
	for _, runtimeUpstream := range config.Upstreams {
		server := strings.TrimSpace(runtimeUpstream.Server)
		if net.ParseIP(server) == nil || runtimeUpstream.Port < 1 || runtimeUpstream.Port > 65535 {
			return Config{}, fmt.Errorf("%w: upstream %q must use an IP literal and valid port", ErrInvalidConfig, runtimeUpstream.Provider)
		}
		upstreams = append(upstreams, Upstream{
			Name:          runtimeUpstream.Provider,
			Network:       runtimeUpstream.Type,
			Address:       net.JoinHostPort(server, strconv.Itoa(runtimeUpstream.Port)),
			Path:          runtimeUpstream.Path,
			TLSServerName: runtimeUpstream.TLSServerName,
		})
	}
	return Config{
		Upstreams:       upstreams,
		AttemptTimeout:  3 * time.Second,
		FailureCooldown: 30 * time.Second,
		MaxRequestBytes: 4096,
		MaxConcurrent:   128,
		Exchange:        exchange,
	}, nil
}

type RunDeps struct {
	LoadRuntimeConfig func(string) (RuntimeConfig, error)
	NewDirectExchange func(DirectExchangeConfig) (ExchangeFunc, error)
	Serve             func(context.Context, ServerConfig, *Forwarder) error
	DiscoverInterface InterfaceDiscoverFunc
}

func defaultRunDeps() RunDeps {
	return RunDeps{
		LoadRuntimeConfig: LoadRuntimeConfig,
		NewDirectExchange: NewDirectExchange,
		Serve:             Serve,
		DiscoverInterface: DiscoverDefaultInterface,
	}
}

// Run is the CLI-callable bootstrap-dns entry point.
func Run(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := RunContext(ctx, args, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func RunContext(ctx context.Context, args []string, stderr io.Writer) error {
	return RunContextWithDeps(ctx, args, stderr, defaultRunDeps())
}

// RunContextWithDeps permits command tests to validate parsing and wiring
// without opening sockets or reading a host route table.
func RunContextWithDeps(ctx context.Context, args []string, stderr io.Writer, deps RunDeps) error {
	if stderr == nil {
		stderr = io.Discard
	}
	flags := flag.NewFlagSet("bootstrap-dns", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "runtime JSON configuration")
	listenAddress := flags.String("listen", DefaultListenAddress, "literal loopback UDP/TCP address")
	tunDevice := flags.String("tun-dev", "singbox", "TUN interface to exclude from physical route discovery")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || strings.TrimSpace(*configPath) == "" {
		return errors.New("bootstrap-dns requires --config and accepts no positional arguments")
	}
	if deps.LoadRuntimeConfig == nil || deps.NewDirectExchange == nil || deps.Serve == nil || deps.DiscoverInterface == nil {
		return fmt.Errorf("%w: incomplete runtime dependencies", ErrInvalidConfig)
	}
	serverConfig := ServerConfig{ListenAddress: strings.TrimSpace(*listenAddress)}
	if err := serverConfig.Validate(); err != nil {
		return err
	}
	runtimeConfig, err := deps.LoadRuntimeConfig(*configPath)
	if err != nil {
		return err
	}
	exchange, err := deps.NewDirectExchange(DirectExchangeConfig{
		ExcludedInterfaces: []string{strings.TrimSpace(*tunDevice)},
		DiscoverInterface:  deps.DiscoverInterface,
		DialTimeout:        3 * time.Second,
	})
	if err != nil {
		return err
	}
	forwarderConfig, err := runtimeConfig.ForwarderConfig(exchange)
	if err != nil {
		return err
	}
	forwarder, err := NewForwarder(forwarderConfig)
	if err != nil {
		return err
	}
	return deps.Serve(ctx, serverConfig, forwarder)
}
