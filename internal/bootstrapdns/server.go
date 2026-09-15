package bootstrapdns

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const DefaultListenAddress = "127.0.0.1:6450"

const (
	serverClientTimeout  = 10 * time.Second
	serverRequestTimeout = 10 * time.Second
)

var ErrNonLoopbackListen = errors.New("bootstrap DNS server must listen on a literal loopback address")

type ServerConfig struct {
	ListenAddress string `json:"listen_address"`
}

func (config ServerConfig) Validate() error {
	host, portText, err := net.SplitHostPort(config.ListenAddress)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrNonLoopbackListen, config.ListenAddress)
	}
	address := net.ParseIP(host)
	port, portErr := strconv.Atoi(portText)
	if address == nil || !address.IsLoopback() || portErr != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%w: %q", ErrNonLoopbackListen, config.ListenAddress)
	}
	return nil
}

type ListenFunc func(network, address string) (net.Listener, error)
type ListenPacketFunc func(network, address string) (net.PacketConn, error)

func ListenTCP(config ServerConfig, listen ListenFunc) (net.Listener, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if listen == nil {
		listen = net.Listen
	}
	return listen("tcp", config.ListenAddress)
}

func ListenUDP(config ServerConfig, listen ListenPacketFunc) (net.PacketConn, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if listen == nil {
		listen = net.ListenPacket
	}
	return listen("udp", config.ListenAddress)
}

type ServerDeps struct {
	ListenTCP ListenFunc
	ListenUDP ListenPacketFunc
}

// Serve starts UDP and TCP DNS listeners on the same validated loopback
// endpoint. Closing ctx stops both listeners and all active TCP connections.
func Serve(ctx context.Context, config ServerConfig, forwarder *Forwarder) error {
	return ServeWithDeps(ctx, config, forwarder, ServerDeps{})
}

// ServeWithDeps is the injectable variant used by tests and embedders.
func ServeWithDeps(ctx context.Context, config ServerConfig, forwarder *Forwarder, deps ServerDeps) error {
	if forwarder == nil {
		return fmt.Errorf("%w: nil forwarder", ErrInvalidConfig)
	}
	tcpListener, err := ListenTCP(config, deps.ListenTCP)
	if err != nil {
		return err
	}
	udpListener, err := ListenUDP(config, deps.ListenUDP)
	if err != nil {
		_ = tcpListener.Close()
		return err
	}

	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		select {
		case <-serveCtx.Done():
			_ = tcpListener.Close()
			_ = udpListener.Close()
		case <-done:
		}
	}()
	errorsCh := make(chan error, 2)
	go func() { errorsCh <- serveTCP(serveCtx, tcpListener, forwarder) }()
	go func() { errorsCh <- serveUDP(serveCtx, udpListener, forwarder) }()
	firstErr := <-errorsCh
	cancel()
	_ = tcpListener.Close()
	_ = udpListener.Close()
	secondErr := <-errorsCh
	close(done)
	if ctx.Err() != nil {
		return nil
	}
	return errors.Join(firstErr, secondErr)
}

func serveUDP(ctx context.Context, conn net.PacketConn, forwarder *Forwarder) error {
	buffer := make([]byte, 65535)
	workers := make(chan struct{}, cap(forwarder.sem))
	for {
		n, address, err := conn.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read UDP query: %w", err)
		}
		if n > forwarder.maxRequestBytes {
			continue
		}
		select {
		case workers <- struct{}{}:
		case <-ctx.Done():
			return nil
		default:
			// Drop overload before copying attacker-controlled datagrams. The
			// loopback client can retry; allocating a SERVFAIL here would make
			// saturation itself a memory-amplification path.
			continue
		}
		query := append([]byte(nil), buffer[:n]...)
		go func(query []byte, address net.Addr) {
			defer func() { <-workers }()
			requestCtx, cancel := context.WithTimeout(ctx, serverRequestTimeout)
			response, resolveErr := forwarder.Resolve(requestCtx, query)
			cancel()
			if resolveErr != nil {
				response = serverFailure(query)
			}
			if response != nil {
				_, _ = conn.WriteTo(response, address)
			}
		}(query, address)
	}
}

func serveTCP(ctx context.Context, listener net.Listener, forwarder *Forwarder) error {
	var connections sync.WaitGroup
	defer connections.Wait()
	workers := make(chan struct{}, cap(forwarder.sem))
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept TCP query: %w", err)
		}
		select {
		case workers <- struct{}{}:
		case <-ctx.Done():
			_ = conn.Close()
			return nil
		default:
			_ = conn.Close()
			continue
		}
		connections.Add(1)
		go func() {
			defer connections.Done()
			defer func() { <-workers }()
			serveTCPConnection(ctx, conn, forwarder)
		}()
	}
}

func serveTCPConnection(ctx context.Context, conn net.Conn, forwarder *Forwarder) {
	defer conn.Close()
	stop := closeOnContext(ctx, conn)
	defer stop()
	for {
		if err := conn.SetDeadline(time.Now().Add(serverClientTimeout)); err != nil {
			return
		}
		var lengthBytes [2]byte
		if _, err := io.ReadFull(conn, lengthBytes[:]); err != nil {
			return
		}
		length := int(binary.BigEndian.Uint16(lengthBytes[:]))
		if length == 0 || length > forwarder.maxRequestBytes {
			return
		}
		query := make([]byte, length)
		if _, err := io.ReadFull(conn, query); err != nil {
			return
		}
		requestCtx, cancel := context.WithTimeout(ctx, serverRequestTimeout)
		response, err := forwarder.Resolve(requestCtx, query)
		cancel()
		if err != nil {
			response = serverFailure(query)
		}
		if response == nil || len(response) > 65535 {
			return
		}
		frame := make([]byte, len(response)+2)
		binary.BigEndian.PutUint16(frame[:2], uint16(len(response)))
		copy(frame[2:], response)
		if _, err := io.Copy(conn, bytes.NewReader(frame)); err != nil {
			return
		}
	}
}

func serverFailure(query []byte) []byte {
	if validateQuery(query) != nil {
		return nil
	}
	response := append([]byte(nil), query...)
	response[2] |= 0x80
	response[3] = response[3]&0xf0 | 0x02
	return response
}
