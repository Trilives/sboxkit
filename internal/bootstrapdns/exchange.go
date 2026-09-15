package bootstrapdns

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const defaultMaxResponseBytes = 65535

// InterfaceDiscoverFunc returns a physical interface to bind for one attempt.
// It is called for every exchange so interface changes do not require a restart.
type InterfaceDiscoverFunc func(context.Context, []string) (string, error)

type DirectExchangeConfig struct {
	Interface          string
	ExcludedInterfaces []string
	DiscoverInterface  InterfaceDiscoverFunc
	DialTimeout        time.Duration
	MaxResponseBytes   int
	TLSConfig          *tls.Config
}

// NewDirectExchange creates an exchange function whose sockets never use an
// HTTP proxy and whose endpoint is always the upstream IP literal.
func NewDirectExchange(config DirectExchangeConfig) (ExchangeFunc, error) {
	if config.DialTimeout < 0 || config.MaxResponseBytes < 0 {
		return nil, ErrInvalidConfig
	}
	maxResponseBytes := config.MaxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	return func(ctx context.Context, upstream Upstream, query []byte) ([]byte, error) {
		normalized, err := normalizeUpstream(upstream, 0)
		if err != nil {
			return nil, err
		}
		upstream = normalized
		interfaceName := strings.TrimSpace(config.Interface)
		if config.DiscoverInterface != nil {
			var err error
			interfaceName, err = config.DiscoverInterface(ctx, append([]string(nil), config.ExcludedInterfaces...))
			if err != nil {
				return nil, fmt.Errorf("discover physical interface: %w", err)
			}
		}
		dialer, err := directDialer(interfaceName, config.DialTimeout)
		if err != nil {
			return nil, err
		}
		switch upstream.Network {
		case "udp":
			return exchangeUDP(ctx, dialer, upstream.Address, query, maxResponseBytes)
		case "tcp":
			return exchangeTCP(ctx, dialer, upstream.Address, query, maxResponseBytes)
		case "https":
			return exchangeHTTPS(ctx, dialer, upstream, query, maxResponseBytes, config.TLSConfig)
		default:
			return nil, fmt.Errorf("%w: unsupported transport %q", ErrInvalidConfig, upstream.Network)
		}
	}, nil
}

// DirectExchange is useful for local integration and callers that do not need
// interface binding. Production callers should use NewDirectExchange with an
// interface or a discovery callback.
func DirectExchange(ctx context.Context, upstream Upstream, query []byte) ([]byte, error) {
	exchange, err := NewDirectExchange(DirectExchangeConfig{})
	if err != nil {
		return nil, err
	}
	return exchange(ctx, upstream, query)
}

func directDialer(interfaceName string, timeout time.Duration) (*net.Dialer, error) {
	dialer := &net.Dialer{Timeout: timeout}
	if interfaceName == "" {
		return dialer, nil
	}
	if _, err := net.InterfaceByName(interfaceName); err != nil {
		return nil, fmt.Errorf("bind interface %q: %w", interfaceName, err)
	}
	dialer.Control = bindToDevice(interfaceName)
	return dialer, nil
}

func exchangeUDP(ctx context.Context, dialer *net.Dialer, address string, query []byte, limit int) ([]byte, error) {
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := closeOnContext(ctx, conn)
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	if n, err := conn.Write(query); err != nil {
		return nil, err
	} else if n != len(query) {
		return nil, io.ErrShortWrite
	}
	buffer := make([]byte, min(limit+1, 65536))
	n, err := conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, ErrResponseTooLarge
	}
	return append([]byte(nil), buffer[:n]...), nil
}

func exchangeTCP(ctx context.Context, dialer *net.Dialer, address string, query []byte, limit int) ([]byte, error) {
	if len(query) > 65535 {
		return nil, ErrRequestTooLarge
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := closeOnContext(ctx, conn)
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	frame := make([]byte, len(query)+2)
	binary.BigEndian.PutUint16(frame[:2], uint16(len(query)))
	copy(frame[2:], query)
	if _, err := io.Copy(conn, bytes.NewReader(frame)); err != nil {
		return nil, err
	}
	var lengthBytes [2]byte
	if _, err := io.ReadFull(conn, lengthBytes[:]); err != nil {
		return nil, err
	}
	length := int(binary.BigEndian.Uint16(lengthBytes[:]))
	if length > limit {
		return nil, ErrResponseTooLarge
	}
	response := make([]byte, length)
	if _, err := io.ReadFull(conn, response); err != nil {
		return nil, err
	}
	return response, nil
}

func exchangeHTTPS(ctx context.Context, dialer *net.Dialer, upstream Upstream, query []byte, limit int, baseTLS *tls.Config) ([]byte, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if baseTLS != nil {
		tlsConfig = baseTLS.Clone()
		if tlsConfig.MinVersion == 0 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	tlsConfig.ServerName = upstream.TLSServerName
	transport := &http.Transport{
		Proxy:             nil,
		DialContext:       dialer.DialContext,
		ForceAttemptHTTP2: true,
		TLSClientConfig:   tlsConfig,
	}
	defer transport.CloseIdleConnections()
	endpoint := "https://" + upstream.Address + upstream.Path
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(query))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/dns-message")
	request.Header.Set("Content-Type", "application/dns-message")
	request.Host = upstream.TLSServerName
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("DoH server returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, ErrResponseTooLarge
	}
	return body, nil
}

func closeOnContext(ctx context.Context, conn net.Conn) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-done:
		}
	}()
	return func() { close(done) }
}

func validateInterfaceName(name string) error {
	if name == "" || strings.ContainsAny(name, "\x00/\r\n") {
		return errors.New("invalid interface name")
	}
	return nil
}
