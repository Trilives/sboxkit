package sysd

import (
	"testing"
	"time"
)

func TestSystemdAndInterfaceNamesRejectDirectiveAndPathInjection(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"sing-box", "name_1.2@x"} {
		if err := validateServiceName(name); err != nil {
			t.Errorf("valid service %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", "-option", "../escape", "bad\nAfter=x", "name/service"} {
		if err := validateServiceName(name); err == nil {
			t.Errorf("unsafe service %q accepted", name)
		}
	}
	for _, name := range []string{"singbox", "wg-0", "eth0.10"} {
		if err := validateInterfaceName(name); err != nil {
			t.Errorf("valid interface %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", "-tun", "../tun", "bad\nname", "interface-name-too-long"} {
		if err := validateInterfaceName(name); err == nil {
			t.Errorf("unsafe interface %q accepted", name)
		}
	}
}

func TestManagedServiceIsExplicitlySingleInstance(t *testing.T) {
	t.Parallel()
	if err := validateManagedServiceName(DefaultName); err != nil {
		t.Fatalf("default managed service rejected: %v", err)
	}
	if err := validateManagedServiceName("custom"); err == nil {
		t.Fatal("custom managed service accepted despite singleton runtime artifacts")
	}
}

func TestHealthcheckInputBudgetAndEndpointsAreBounded(t *testing.T) {
	t.Parallel()
	valid := func(url string, attempts int, timeout, gap time.Duration) error {
		return validateHealthcheckInputs("sing-box", "singbox", "timer", url, attempts, timeout, gap, time.Minute)
	}
	if err := valid("https://example.com/generate_204", 3, 8*time.Second, 4*time.Second); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		url      string
		attempts int
		timeout  time.Duration
		gap      time.Duration
	}{
		{url: "file:///etc/passwd", attempts: 1, timeout: time.Second},
		{url: "https://user:pass@example.com/", attempts: 1, timeout: time.Second},
		{url: "https://example.com/", attempts: 11, timeout: time.Second},
		{url: "https://example.com/", attempts: 10, timeout: 30 * time.Second, gap: 30 * time.Second},
	} {
		if err := valid(test.url, test.attempts, test.timeout, test.gap); err == nil {
			t.Errorf("unsafe healthcheck options accepted: %#v", test)
		}
	}
	for _, address := range []string{"127.0.0.1:2080", "[::1]:2080"} {
		if err := validateLoopbackProxyAddress(address); err != nil {
			t.Errorf("valid proxy %q rejected: %v", address, err)
		}
	}
	for _, address := range []string{"localhost:2080", "0.0.0.0:2080", "192.0.2.1:2080", "127.0.0.1:0", "bad"} {
		if err := validateLoopbackProxyAddress(address); err == nil {
			t.Errorf("unsafe proxy %q accepted", address)
		}
	}
}
