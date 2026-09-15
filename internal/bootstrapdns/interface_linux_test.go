//go:build linux

package bootstrapdns

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const routeHeader = "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n"

func TestDiscoverDefaultInterfaceFromReaderChoosesLowestMetricPhysicalRoute(t *testing.T) {
	t.Parallel()
	routes := routeHeader +
		"singbox 00000000 00000000 0001 0 0 0 00000000 0 0 0\n" +
		"eth1 00000000 0100000A 0001 0 0 50 00000000 0 0 0\n" +
		"wlan0 00000000 0101A8C0 0003 0 0 10 00000000 0 0 0\n" +
		"lo 00000000 00000000 0001 0 0 1 00000000 0 0 0\n"
	got, err := discoverDefaultInterfaceFromReader(context.Background(), strings.NewReader(routes), []string{"singbox"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "wlan0" {
		t.Fatalf("interface = %q, want wlan0", got)
	}
}

func TestDiscoverDefaultInterfaceFromReaderRejectsUnusableRoutes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		routes string
	}{
		{name: "no default", routes: routeHeader + "eth0 0002A8C0 00000000 0001 0 0 1 00FFFFFF 0 0 0\n"},
		{name: "down", routes: routeHeader + "eth0 00000000 00000000 0000 0 0 1 00000000 0 0 0\n"},
		{name: "wrong mask", routes: routeHeader + "eth0 00000000 00000000 0001 0 0 1 00FFFFFF 0 0 0\n"},
		{name: "malformed", routes: routeHeader + "eth0 00000000\n"},
		{name: "excluded", routes: routeHeader + "eth0 00000000 00000000 0001 0 0 1 00000000 0 0 0\n"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			excluded := []string(nil)
			if test.name == "excluded" {
				excluded = []string{" eth0 "}
			}
			_, err := discoverDefaultInterfaceFromReader(context.Background(), strings.NewReader(test.routes), excluded)
			if !errors.Is(err, ErrNoPhysicalInterface) {
				t.Fatalf("error = %v, want ErrNoPhysicalInterface", err)
			}
		})
	}
}

func TestDiscoverDefaultInterfaceFromReaderHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := discoverDefaultInterfaceFromReader(ctx, strings.NewReader(routeHeader), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
