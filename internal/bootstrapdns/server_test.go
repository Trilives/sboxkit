package bootstrapdns

import (
	"errors"
	"net"
	"testing"
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

type stubListener struct{}

func (*stubListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (*stubListener) Close() error              { return nil }
func (*stubListener) Addr() net.Addr            { return stubAddr("127.0.0.1:5353") }

type stubAddr string

func (stubAddr) Network() string  { return "tcp" }
func (a stubAddr) String() string { return string(a) }

// Keep the compile-time interface check close to the fake; no socket is opened.
var _ net.Listener = (*stubListener)(nil)
