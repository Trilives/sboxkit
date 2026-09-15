package bootstrapdns

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDirectExchangeUDPAndTCP(t *testing.T) {
	t.Parallel()

	query := dnsQuery(0x1234)
	want := dnsResponse(query)
	for _, network := range []string{"udp", "tcp"} {
		network := network
		t.Run(network, func(t *testing.T) {
			t.Parallel()
			address, closeServer := startDNSExchangeFixture(t, network)
			defer closeServer()
			exchange, err := NewDirectExchange(DirectExchangeConfig{DialTimeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := exchange(ctx, Upstream{Network: network, Address: address}, query)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("response = %v, want %v", got, want)
			}
		})
	}
}

func TestDirectExchangeConvenienceWrapper(t *testing.T) {
	t.Parallel()
	address, closeServer := startDNSExchangeFixture(t, "udp")
	defer closeServer()
	query := dnsQuery(0x4321)
	response, err := DirectExchange(context.Background(), Upstream{Network: "udp", Address: address}, query)
	if err != nil {
		t.Fatal(err)
	}
	if want := dnsResponse(query); !reflect.DeepEqual(response, want) {
		t.Fatalf("response = %v, want %v", response, want)
	}
}

func TestDirectExchangeHTTPSUsesLiteralIPWithTLSName(t *testing.T) {
	t.Parallel()

	query := dnsQuery(0x2468)
	want := dnsResponse(query)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/dns-query" || request.Header.Get("Content-Type") != "application/dns-message" {
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil || !reflect.DeepEqual(raw, query) {
			http.Error(writer, "bad DNS payload", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/dns-message")
		_, _ = writer.Write(want)
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	exchange, err := NewDirectExchange(DirectExchangeConfig{
		DialTimeout: time.Second,
		TLSConfig:   &tls.Config{RootCAs: roots},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := exchange(context.Background(), Upstream{
		Network: "https", Address: parsed.Host, Path: "/dns-query", TLSServerName: "example.com",
	}, query)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %v, want %v", got, want)
	}
}

func TestDirectExchangeEnforcesConfigurationAndResponseBounds(t *testing.T) {
	t.Parallel()

	for _, config := range []DirectExchangeConfig{{DialTimeout: -1}, {MaxResponseBytes: -1}} {
		if _, err := NewDirectExchange(config); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("NewDirectExchange(%#v) error = %v", config, err)
		}
	}
	exchange, err := NewDirectExchange(DirectExchangeConfig{Interface: "interface-that-does-not-exist", DialTimeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = exchange(context.Background(), Upstream{Network: "udp", Address: "192.0.2.1:53"}, dnsQuery(1))
	if err == nil || !strings.Contains(err.Error(), "bind interface") {
		t.Fatalf("invalid interface error = %v", err)
	}
	for _, name := range []string{"", "bad/name", "bad\nname", "bad\x00name"} {
		if err := validateInterfaceName(name); err == nil {
			t.Errorf("validateInterfaceName(%q) error = nil", name)
		}
	}
	if err := validateInterfaceName("eth0"); err != nil {
		t.Fatalf("validateInterfaceName(eth0) error = %v", err)
	}
}

func startDNSExchangeFixture(t *testing.T, network string) (string, func()) {
	t.Helper()
	if network == "udp" {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			buffer := make([]byte, 4096)
			n, address, readErr := conn.ReadFrom(buffer)
			if readErr == nil {
				_, _ = conn.WriteTo(dnsResponse(buffer[:n]), address)
			}
		}()
		return conn.LocalAddr().String(), func() { _ = conn.Close() }
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		var lengthBytes [2]byte
		if _, readErr := io.ReadFull(conn, lengthBytes[:]); readErr != nil {
			return
		}
		query := make([]byte, int(binary.BigEndian.Uint16(lengthBytes[:])))
		if _, readErr := io.ReadFull(conn, query); readErr != nil {
			return
		}
		response := dnsResponse(query)
		binary.BigEndian.PutUint16(lengthBytes[:], uint16(len(response)))
		_, _ = conn.Write(append(lengthBytes[:], response...))
	}()
	return listener.Addr().String(), func() { _ = listener.Close() }
}
