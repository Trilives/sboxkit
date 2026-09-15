package bootstrapdns

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestServerConfigAcceptsOnlyLiteralLoopbackAddresses(t *testing.T) {
	t.Parallel()

	for _, address := range []string{"127.0.0.1:53", "127.9.8.7:5353", "[::1]:53"} {
		address := address
		t.Run("accept "+address, func(t *testing.T) {
			t.Parallel()
			if err := (ServerConfig{ListenAddress: address}).Validate(); err != nil {
				t.Fatalf("ServerConfig.Validate(%q) error = %v", address, err)
			}
		})
	}

	for _, address := range []string{
		"0.0.0.0:53",
		"[::]:53",
		"192.168.1.10:53",
		"localhost:53",
		":53",
		"127.0.0.1",
	} {
		address := address
		t.Run("reject "+address, func(t *testing.T) {
			t.Parallel()
			if err := (ServerConfig{ListenAddress: address}).Validate(); !errors.Is(err, ErrNonLoopbackListen) {
				t.Fatalf("ServerConfig.Validate(%q) error = %v, want ErrNonLoopbackListen", address, err)
			}
		})
	}
}

func TestListenTCPValidatesAddressBeforeCallingInjectedListener(t *testing.T) {
	t.Parallel()

	called := false
	listen := func(string, string) (net.Listener, error) {
		called = true
		return &stubListener{}, nil
	}

	if _, err := ListenTCP(ServerConfig{ListenAddress: "0.0.0.0:53"}, listen); !errors.Is(err, ErrNonLoopbackListen) {
		t.Fatalf("ListenTCP() error = %v, want ErrNonLoopbackListen", err)
	}
	if called {
		t.Fatal("injected listener called for a non-loopback address")
	}
}

func TestListenTCPUsesValidatedLoopbackAddress(t *testing.T) {
	t.Parallel()

	var gotNetwork, gotAddress string
	wantListener := &stubListener{}
	listener, err := ListenTCP(ServerConfig{ListenAddress: "127.0.0.1:5353"}, func(network, address string) (net.Listener, error) {
		gotNetwork = network
		gotAddress = address
		return wantListener, nil
	})
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}
	if listener != wantListener {
		t.Fatalf("ListenTCP() listener = %T %p, want injected listener %p", listener, listener, wantListener)
	}
	if gotNetwork != "tcp" || gotAddress != "127.0.0.1:5353" {
		t.Fatalf("injected listener called with (%q, %q), want (%q, %q)", gotNetwork, gotAddress, "tcp", "127.0.0.1:5353")
	}
}

func TestDefaultTCPAndUDPListenersBindOnlyLoopback(t *testing.T) {
	t.Parallel()

	tcpProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcpAddress := tcpProbe.Addr().String()
	_ = tcpProbe.Close()
	tcpListener, err := ListenTCP(ServerConfig{ListenAddress: tcpAddress}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = tcpListener.Close()

	udpProbe, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpAddress := udpProbe.LocalAddr().String()
	_ = udpProbe.Close()
	udpListener, err := ListenUDP(ServerConfig{ListenAddress: udpAddress}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = udpListener.Close()
}

func TestServeWithDepsAnswersUDPAndTCPAndStopsWithContext(t *testing.T) {
	t.Parallel()

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := tcpListener.Addr().(*net.TCPAddr).Port
	udpConn, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		_ = tcpListener.Close()
		t.Fatal(err)
	}
	query := dnsQuery(0x5588)
	forwarder := newTestForwarder(t, func(_ context.Context, _ Upstream, request []byte) ([]byte, error) {
		return dnsResponse(request), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	config := ServerConfig{ListenAddress: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	go func() {
		done <- ServeWithDeps(ctx, config, forwarder, ServerDeps{
			ListenTCP: func(string, string) (net.Listener, error) { return tcpListener, nil },
			ListenUDP: func(string, string) (net.PacketConn, error) { return udpConn, nil },
		})
	}()

	udpClient, err := net.DialTimeout("udp", config.ListenAddress, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := udpClient.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := udpClient.Write(query); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4096)
	n, err := udpClient.Read(buffer)
	_ = udpClient.Close()
	if err != nil {
		t.Fatal(err)
	}
	if want := dnsResponse(query); !reflect.DeepEqual(buffer[:n], want) {
		t.Fatalf("UDP response = %v, want %v", buffer[:n], want)
	}

	tcpClient, err := net.DialTimeout("tcp", config.ListenAddress, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := tcpClient.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, len(query)+2)
	binary.BigEndian.PutUint16(frame[:2], uint16(len(query)))
	copy(frame[2:], query)
	if _, err := tcpClient.Write(frame); err != nil {
		t.Fatal(err)
	}
	var lengthBytes [2]byte
	if _, err := io.ReadFull(tcpClient, lengthBytes[:]); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, int(binary.BigEndian.Uint16(lengthBytes[:])))
	if _, err := io.ReadFull(tcpClient, response); err != nil {
		t.Fatal(err)
	}
	if want := dnsResponse(query); !reflect.DeepEqual(response, want) {
		t.Fatalf("TCP response = %v, want %v", response, want)
	}

	cancel()
	_ = tcpClient.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServeWithDeps() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeWithDeps did not stop after context cancellation")
	}
}

func TestServeWrapperStopsOnAnAlreadyCanceledContext(t *testing.T) {
	t.Parallel()

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	_ = probe.Close()
	forwarder := newTestForwarder(t, func(_ context.Context, _ Upstream, request []byte) ([]byte, error) {
		return dnsResponse(request), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Serve(ctx, ServerConfig{ListenAddress: address}, forwarder); err != nil {
		t.Fatalf("Serve(canceled) error = %v", err)
	}
}

func TestServeWithDepsReturnsServerFailureWhenAllUpstreamsFail(t *testing.T) {
	t.Parallel()

	query := dnsQuery(0x7788)
	forwarder := newTestForwarder(t, func(context.Context, Upstream, []byte) ([]byte, error) {
		return nil, errors.New("injected upstream failure")
	})
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		serveTCPConnection(context.Background(), server, forwarder)
		close(done)
	}()
	if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, len(query)+2)
	binary.BigEndian.PutUint16(frame[:2], uint16(len(query)))
	copy(frame[2:], query)
	if _, err := client.Write(frame); err != nil {
		t.Fatal(err)
	}
	var lengthBytes [2]byte
	if _, err := io.ReadFull(client, lengthBytes[:]); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, int(binary.BigEndian.Uint16(lengthBytes[:])))
	if _, err := io.ReadFull(client, response); err != nil {
		t.Fatal(err)
	}
	if response[2]&0x80 == 0 || response[3]&0x0f != 2 {
		t.Fatalf("response flags = %#x %#x, want QR + SERVFAIL", response[2], response[3])
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TCP handler did not stop when client closed")
	}
}

func TestServerFailureRejectsInvalidQuery(t *testing.T) {
	t.Parallel()
	if response := serverFailure([]byte{1, 2, 3}); response != nil {
		t.Fatalf("serverFailure(invalid) = %v, want nil", response)
	}
}

type stubListener struct{}

func (*stubListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (*stubListener) Close() error              { return nil }
func (*stubListener) Addr() net.Addr            { return stubAddr("127.0.0.1:5353") }

type stubAddr string

func (stubAddr) Network() string  { return "tcp" }
func (a stubAddr) String() string { return string(a) }

// Keep the compile-time interface check close to the fake; no socket is opened.
var _ net.Listener = (*stubListener)(nil)
