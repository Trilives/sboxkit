package config

import "testing"

func TestSetFieldUpdatesBoolStringIntAndList(t *testing.T) {
	cfg := Defaults()
	if err := SetField(&cfg, "enable_tun", "false"); err != nil {
		t.Fatalf("set bool: %v", err)
	}
	if err := SetField(&cfg, "bootstrap_dns_port", "5353"); err != nil {
		t.Fatalf("set int: %v", err)
	}
	if err := SetField(&cfg, "download_proxy", "http://127.0.0.1:7890"); err != nil {
		t.Fatalf("set string: %v", err)
	}
	if err := SetField(&cfg, "direct_domain_suffixes", "example.com,example.org"); err != nil {
		t.Fatalf("set list: %v", err)
	}

	if cfg.EnableTun {
		t.Fatal("expected enable_tun false")
	}
	if cfg.BootstrapDNSPort != 5353 {
		t.Fatalf("unexpected port %d", cfg.BootstrapDNSPort)
	}
	if cfg.DownloadProxy != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected proxy %q", cfg.DownloadProxy)
	}
	if len(cfg.DirectDomainSuffixes) != 2 {
		t.Fatalf("unexpected suffixes %#v", cfg.DirectDomainSuffixes)
	}
}
