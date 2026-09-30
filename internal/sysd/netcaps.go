// 用户模式下 sing-box 以普通用户身份运行。TUN 入站（以及 routing_mark /
// default_mark 的 SO_MARK）需要 CAP_NET_ADMIN：只在运行时配置确实需要时，经 sudo
// 用 setcap 给运行时内核授予与系统单元相同的能力集。能力保存在内核文件本身，之后
// 的重启、同步与 watchdog 重启都无需再提权，直到内核二进制被替换。
package sysd

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

// netCapabilities 与系统单元的 AmbientCapabilities 保持一致。
const netCapabilities = "cap_net_admin,cap_net_bind_service,cap_net_raw+ep"

const (
	capNetBindService = 10
	capNetAdmin       = 12
	capNetRaw         = 13

	vfsCapFlagEffective = 0x000001
	vfsCapRevisionMask  = 0xFF000000
	vfsCapRevision1     = 0x01000000
	vfsCapRevision2     = 0x02000000
	vfsCapRevision3     = 0x03000000
)

var requiredCapBits = []uint{capNetBindService, capNetAdmin, capNetRaw}

// configNeedsNetAdmin 判断运行时配置是否用到需要 CAP_NET_ADMIN 的选项。
func configNeedsNetAdmin(doc map[string]any) bool {
	if inbounds, ok := doc["inbounds"].([]any); ok {
		for _, item := range inbounds {
			if inbound, _ := item.(map[string]any); inbound != nil && inbound["type"] == "tun" {
				return true
			}
		}
	}
	if outbounds, ok := doc["outbounds"].([]any); ok {
		for _, item := range outbounds {
			if outbound, _ := item.(map[string]any); outbound != nil && outbound["routing_mark"] != nil {
				return true
			}
		}
	}
	route, _ := doc["route"].(map[string]any)
	return route != nil && route["default_mark"] != nil
}

func runtimeConfigNeedsNetAdmin(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false, fmt.Errorf("parse runtime config %s: %w", path, err)
	}
	return configNeedsNetAdmin(doc), nil
}

// parseFileCaps 解析 security.capability xattr（struct vfs_cap_data，v1–v3），
// 返回 effective 标志与 permitted 位图。
func parseFileCaps(raw []byte) (effective bool, permitted uint64, ok bool) {
	if len(raw) < 12 {
		return false, 0, false
	}
	magic := binary.LittleEndian.Uint32(raw[0:4])
	permitted = uint64(binary.LittleEndian.Uint32(raw[4:8]))
	switch magic & vfsCapRevisionMask {
	case vfsCapRevision1:
	case vfsCapRevision2, vfsCapRevision3:
		if len(raw) < 20 {
			return false, 0, false
		}
		permitted |= uint64(binary.LittleEndian.Uint32(raw[12:16])) << 32
	default:
		return false, 0, false
	}
	return magic&vfsCapFlagEffective != 0, permitted, true
}

func capsSatisfied(raw []byte) bool {
	effective, permitted, ok := parseFileCaps(raw)
	if !ok || !effective {
		return false
	}
	for _, bit := range requiredCapBits {
		if permitted&(1<<bit) == 0 {
			return false
		}
	}
	return true
}

func hasNetCapabilities(path string) bool {
	buf := make([]byte, 64)
	n, err := unix.Getxattr(path, "security.capability", buf)
	return err == nil && capsSatisfied(buf[:n])
}

// mountedNosuid 文件能力在 nosuid 挂载上会被内核忽略：setcap 成功但服务仍无权限。
func mountedNosuid(path string) bool {
	var st unix.Statfs_t
	if err := unix.Statfs(filepath.Dir(path), &st); err != nil {
		return false
	}
	return st.Flags&unix.ST_NOSUID != 0
}

func findSetcap() string {
	if path, err := exec.LookPath("setcap"); err == nil {
		return path
	}
	for _, candidate := range []string{"/usr/sbin/setcap", "/sbin/setcap"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// ensureNetCapabilities 用户模式下按需为运行时内核授予网络管理能力（TUN 时请求 sudo）。
// 系统模式由 unit 的 AmbientCapabilities 负责，这里不做任何事。
func ensureNetCapabilities(rt runtimePaths) error {
	if !rt.Scope.User {
		return nil
	}
	need, err := runtimeConfigNeedsNetAdmin(rt.Config)
	if err != nil || !need || hasNetCapabilities(rt.Bin) {
		return err
	}
	return grantNetCapabilities(rt.Bin)
}

// grantNetCapabilities 经 sudo setcap 给 bin 授予网络能力并复核。
func grantNetCapabilities(bin string) error {
	if mountedNosuid(bin) {
		return fmt.Errorf(i18n.T("运行时目录 %s 所在文件系统以 nosuid 挂载，TUN 所需的文件能力不会生效；请把用户模式安装包放到其他位置"), filepath.Dir(bin))
	}
	setcap := findSetcap()
	if setcap == "" {
		return errors.New(i18n.T("启用 TUN 需要 setcap 命令（Debian/Ubuntu: libcap2-bin，Fedora/Arch: libcap）"))
	}
	execx.Info(i18n.T("配置启用了 TUN：将通过 sudo 为运行时 sing-box 授予网络管理能力（setcap），之后重启无需再次授权。"))
	if _, err := execx.RunRoot([]string{setcap, netCapabilities, bin}, i18n.T("为 sing-box 授予 TUN 所需能力"), nil); err != nil {
		return err
	}
	if !hasNetCapabilities(bin) {
		return fmt.Errorf(i18n.T("为 %s 授予网络能力失败：文件系统可能不支持扩展属性"), bin)
	}
	return nil
}
