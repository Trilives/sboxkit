// 系统模式 sing-box 的运行身份。默认以 systemd 动态分配的非特权用户（DynamicUser）运行，
// 只保留 TUN / 路由所需的网络能力：配置由 systemd 以 root 读取后经 LoadCredential 交付，
// 文件系统整体只读（ProtectSystem=strict），可写的只有 StateDirectory 下的缓存，订阅或
// 原生配置里的文件路径因此无法再借 root 读写任意文件。TUN 链路的系统 DNS 由单元以 root
// 执行的 ExecStartPost 代为登记（见 tundns.go）。以下情形仍以 root 运行：
//   - 配置按进程名 / 路径分流：识别其他用户（含 root 守护进程）的连接需要读取其 /proc 信息；
//   - 无法确定 TUN 的链路名或 DNS 地址，只能由 sing-box 自行登记；
//   - systemd 早于 247，不支持 LoadCredential；
//   - 本版本切换为非特权身份后服务没能启动、改用 root 却能运行（见 verifyIdentitySwitch）。
package sysd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

type serviceIdentity int

const (
	identityRoot serviceIdentity = iota
	identityDynamic
)

const (
	// serviceStateDirName sing-box 唯一可写的目录（/var/lib/<name>，DynamicUser 时由 systemd
	// 放在 /var/lib/private 下并交给动态用户）。
	serviceStateDirName = "sboxkit-cache"
	// minCredentialSystemd LoadCredential= 自 systemd 247 起可用。
	minCredentialSystemd = 247
	// identityFallbackFile 记录本版本已退回 root 运行（内容为版本号）；版本变化后重新尝试。
	identityFallbackFile = "runtime.identity-fallback"
	// identitySettle 切换身份后等待服务稳定的时间：Restart=on-failure 的 RestartSec=3。
	identitySettle = 4 * time.Second
)

// 可在测试中替换。
var (
	systemdVersion = detectSystemdVersion
	settleDelay    = identitySettle
	unitHealthy    = unitSettles
	unitProps      = mainUnitProps
	rewriteUnit    = rewriteAndRestart
)

// processRuleKeys 需要识别连接所属进程的规则字段。
var processRuleKeys = []string{"process_name", "process_path", "process_path_regex"}

// chooseIdentity 按运行时配置决定系统单元的身份；reason 非空时说明为何仍以 root 运行。
func chooseIdentity(p paths.Paths, s unitScope, doc map[string]any) (serviceIdentity, string) {
	if s.User {
		return identityRoot, ""
	}
	if usesProcessRules(doc) {
		return identityRoot, i18n.T("配置按进程名分流，识别其他用户的进程需要 root，sing-box 继续以 root 运行")
	}
	links, err := tunDNSLinks(doc)
	if err != nil {
		return identityRoot, fmt.Sprintf(i18n.T("%v，sing-box 继续以 root 运行，由它自行登记系统 DNS"), err)
	}
	// 每个链路一行 ExecStartPost，依次各等至多 1 分钟，多个链路可能超出服务的启动时限。
	if len(links) > 1 {
		return identityRoot, i18n.T("配置含多个 TUN 入站，sing-box 继续以 root 运行，由它自行登记系统 DNS")
	}
	if v := systemdVersion(); v < minCredentialSystemd {
		return identityRoot, fmt.Sprintf(i18n.T("systemd %d 不支持 LoadCredential（需要 %d+），sing-box 继续以 root 运行"), v, minCredentialSystemd)
	}
	if fellBack(p) {
		return identityRoot, i18n.T("此前以非 root 身份启动失败，本版本内 sing-box 继续以 root 运行")
	}
	return identityDynamic, ""
}

// usesProcessRules 在配置任意位置（路由 / DNS 规则、逻辑规则、内联规则集）查找进程规则。
func usesProcessRules(v any) bool {
	switch node := v.(type) {
	case map[string]any:
		for _, key := range processRuleKeys {
			if hasValues(node[key]) {
				return true
			}
		}
		for _, child := range node {
			if usesProcessRules(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if usesProcessRules(child) {
				return true
			}
		}
	}
	return false
}

// hasValues 规则字段是否给出了条件（非空字符串或非空列表）。
func hasValues(v any) bool {
	switch val := v.(type) {
	case string:
		return val != ""
	case []any:
		return len(val) > 0
	}
	return false
}

func detectSystemdVersion() int {
	out, err := exec.Command("systemctl", "--version").Output()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 || fields[0] != "systemd" {
		return 0
	}
	v, _ := strconv.Atoi(fields[1])
	return v
}

func fallbackMarker(p paths.Paths) string { return filepath.Join(p.State, identityFallbackFile) }

func fellBack(p paths.Paths) bool {
	raw, err := os.ReadFile(fallbackMarker(p))
	return err == nil && strings.TrimSpace(string(raw)) == Version
}

// serviceCacheDB 系统单元的缓存文件：在 StateDirectory 中，与运行身份无关。
func serviceCacheDB() string {
	return filepath.Join("/var/lib", serviceStateDirName, "cache.db")
}

// needsIdentityCheck 新单元以非特权身份运行且与已安装的单元不同（从 root 切换过来，或代为
// 登记 TUN DNS 的 ExecStartPost 有变）时，重启后要确认服务稳定运行。
func needsIdentityCheck(name string, rt runtimePaths) bool {
	if rt.Identity != identityDynamic {
		return false
	}
	installed, err := os.ReadFile(rt.Unit)
	if err != nil {
		return true
	}
	want, err := renderUnit(name, rt)
	return err != nil || string(installed) != want
}

// unitSettles 刚重启的服务等待 settleDelay 后仍处于 active，且这段时间没有被 Restart=on-failure
// 自动重启。只比较等待期间的变化：服务正在等待自动重启时，systemctl restart 不清零 NRestarts。
func unitSettles(s unitScope, name string) bool {
	before, err := unitProps(s, name)
	if err != nil {
		return false
	}
	time.Sleep(settleDelay)
	after, err := unitProps(s, name)
	return err == nil && after["ActiveState"] == "active" && after["NRestarts"] == before["NRestarts"]
}

func mainUnitProps(s unitScope, name string) (map[string]string, error) {
	cmd := s.systemctl("show", "-p", "ActiveState", "-p", "NRestarts", name+".service")
	out, err := exec.Command(cmd[0], cmd[1:]...).Output()
	if err != nil {
		return nil, err
	}
	props := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[k] = v
		}
	}
	return props, nil
}

// verifyIdentitySwitch 主单元刚从 root 切到非特权身份并重启后，确认服务稳定运行。restartErr 是
// 这次重启的结果：代为登记 TUN DNS 的 ExecStartPost 失败时启动任务本身失败，同样算没能启动。
// 没能启动时改写为 root 单元重启：root 能稳定运行才说明是身份所致，记下本版本已退回（避免每次
// 同步都重试）；root 同样起不来说明故障与身份无关（端口占用、节点配置等），恢复非特权单元且不
// 记录，以免一次无关故障让 sing-box 在整个版本内失去沙箱，并报错交给调用方回退到切换前的配置。
func verifyIdentitySwitch(p paths.Paths, name string, rt runtimePaths, restartErr error) error {
	if restartErr == nil && unitHealthy(rt.Scope, name) {
		return nil
	}
	execx.Warn(i18n.T("sing-box 以非 root 身份未能稳定运行，改回 root 运行（原因见 journalctl -u sing-box）。"))
	asRoot := rt
	asRoot.Identity = identityRoot
	if err := rewriteUnit(name, asRoot); err != nil {
		return err
	}
	if !unitHealthy(rt.Scope, name) {
		failed := errors.New(i18n.T("sing-box 以 root 运行同样失败，故障与运行身份无关（原因见 journalctl -u sing-box）"))
		return errors.Join(failed, rewriteUnit(name, rt))
	}
	if err := os.WriteFile(fallbackMarker(p), []byte(Version+"\n"), 0o644); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("记录运行身份失败：%v"), err))
	}
	return nil
}

func rewriteAndRestart(name string, rt runtimePaths) error {
	if err := writeMainUnit(name, rt); err != nil {
		return err
	}
	return restartMain(rt.Scope, name, "")
}
