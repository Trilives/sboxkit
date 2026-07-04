// Package firewall 防火墙放行（对应 firewall.py）：开启局域网代理时放行混合入站端口。
// 自动探测本机防火墙工具（ufw > firewalld > nftables > iptables），以 root 增删规则；
// 探测不到任何工具时给出手动提示，不报错。
package firewall

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

const ProxyPort = 7890

var protocols = []string{"tcp", "udp"}

// Detect 返回当前可用的防火墙后端名；都没有则空串。
func Detect() string {
	for _, b := range []struct{ cmd, name string }{
		{"ufw", "ufw"},
		{"firewall-cmd", "firewalld"},
		{"nft", "nftables"},
		{"iptables", "iptables"},
	} {
		if execx.Have(b.cmd) {
			return b.name
		}
	}
	return ""
}

// Allow 放行端口。成功应用返回 true；无可用工具返回 false。
func Allow(port int) bool {
	backend := Detect()
	if backend == "" {
		execx.Warn(fmt.Sprintf(i18n.T("未探测到防火墙工具，请自行确认放行 %d/tcp,udp（或本机无防火墙）。"), port))
		return false
	}
	execx.Info(fmt.Sprintf(i18n.T("经 %s 放行 %d/tcp,udp …"), backend, port))
	dispatch(backend, true, port)
	execx.Ok(fmt.Sprintf(i18n.T("已放行 %d 端口（%s）。"), port, backend))
	return true
}

// Revoke 撤销放行（回退用）。尽力而为，失败不抛。
func Revoke(port int) {
	backend := Detect()
	if backend == "" {
		return
	}
	execx.Info(fmt.Sprintf(i18n.T("经 %s 撤销放行 %d …"), backend, port))
	dispatch(backend, false, port)
}

func dispatch(backend string, add bool, port int) {
	switch backend {
	case "ufw":
		action := []string{"allow"}
		if !add {
			action = []string{"delete", "allow"}
		}
		for _, proto := range protocols {
			args := append(append([]string{"ufw"}, action...), fmt.Sprintf("%d/%s", port, proto))
			execx.RunRoot(args, i18n.T("更新防火墙"), nil)
		}
	case "firewalld":
		flag := "--add-port"
		if !add {
			flag = "--remove-port"
		}
		for _, proto := range protocols {
			execx.RunRoot([]string{"firewall-cmd", "--permanent", fmt.Sprintf("%s=%d/%s", flag, port, proto)}, i18n.T("更新防火墙"), nil)
		}
		execx.RunRoot([]string{"firewall-cmd", "--reload"}, i18n.T("更新防火墙"), nil)
	case "nftables":
		// nftables 无稳定的「删除某条规则」简易命令，这里仅做新增并提示
		if add {
			for _, proto := range protocols {
				execx.RunRoot([]string{"nft", "add", "rule", "inet", "filter", "input",
					proto, "dport", fmt.Sprint(port), "accept"}, i18n.T("更新防火墙"), nil)
			}
		} else {
			execx.Warn(i18n.T("nftables 规则请手动移除：nft -a list chain inet filter input 查看句柄后 delete。"))
		}
	case "iptables":
		op := "-I" // 插入到 INPUT 顶部
		if !add {
			op = "-D"
		}
		for _, proto := range protocols {
			execx.RunRoot([]string{"iptables", op, "INPUT", "-p", proto, "--dport", fmt.Sprint(port), "-j", "ACCEPT"}, i18n.T("更新防火墙"), nil)
		}
	}
}
