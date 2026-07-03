package system

import (
	"context"
	"os/exec"
	"strconv"
)

const ProxyPort = 7890

func DetectFirewall() string {
	for _, candidate := range []struct {
		name string
		cmd  string
	}{
		{"ufw", "ufw"},
		{"firewalld", "firewall-cmd"},
		{"nftables", "nft"},
		{"iptables", "iptables"},
	} {
		if _, err := exec.LookPath(candidate.cmd); err == nil {
			return candidate.name
		}
	}
	return ""
}

func AllowFirewallPort(ctx context.Context, runner Runner, backend string, port int) error {
	return firewallPort(ctx, runner, backend, port, true)
}

func RevokeFirewallPort(ctx context.Context, runner Runner, backend string, port int) error {
	return firewallPort(ctx, runner, backend, port, false)
}

func firewallPort(ctx context.Context, runner Runner, backend string, port int, add bool) error {
	if backend == "" {
		backend = DetectFirewall()
	}
	if backend == "" {
		return nil
	}
	portText := strconv.Itoa(port)
	for _, proto := range []string{"tcp", "udp"} {
		switch backend {
		case "ufw":
			args := []string{"allow", portText + "/" + proto}
			if !add {
				args = []string{"delete", "allow", portText + "/" + proto}
			}
			if err := runner.Run(ctx, "ufw", args...); err != nil {
				return err
			}
		case "firewalld":
			flag := "--add-port=" + portText + "/" + proto
			if !add {
				flag = "--remove-port=" + portText + "/" + proto
			}
			if err := runner.Run(ctx, "firewall-cmd", "--permanent", flag); err != nil {
				return err
			}
		case "nftables":
			if add {
				if err := runner.Run(ctx, "nft", "add", "rule", "inet", "filter", "input", proto, "dport", portText, "accept"); err != nil {
					return err
				}
			}
		case "iptables":
			op := "-I"
			if !add {
				op = "-D"
			}
			if err := runner.Run(ctx, "iptables", op, "INPUT", "-p", proto, "--dport", portText, "-j", "ACCEPT"); err != nil {
				return err
			}
		}
	}
	if backend == "firewalld" {
		return runner.Run(ctx, "firewall-cmd", "--reload")
	}
	return nil
}
