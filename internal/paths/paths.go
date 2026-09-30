// Package paths 统一路径常量。
//
// 运行期所有数据使用**固定工作目录** /var/lib/sboxkit（不随用户/HOME 变化，
// root 运行的定时器与用户会话看到同一份数据）；环境变量 SBOXKIT_HOME 可覆盖
// （主要用于测试）。首次使用由 flows.EnsureStateRoot 负责创建并交回调用者属主。
//
// 用户模式（可执行文件旁存在 sboxkit.user-mode 标记，见 userMode.go）下状态默认
// 落在包目录的 .sboxkit/，运行时暂存于状态目录内，服务以 systemd 用户单元运行。
package paths

import (
	"os"
	"path/filepath"
)

// DefaultStateRoot 固定数据目录。
const DefaultStateRoot = "/var/lib/sboxkit"

// RuntimeDir 系统侧运行时目录：sing-box 服务实际读取的暂存副本
// （内核 + 配置 + geo 规则集 + UI），与状态目录解耦。
const RuntimeDir = "/var/lib/sboxkit-runtime"

// Paths 全部运行期路径；由 Detect 依据环境变量解析一次后传递使用。
type Paths struct {
	UserMode       bool   // 用户模式：systemd 用户单元、无 sudo
	BundleRoot     string // 用户模式安装包目录（含 sboxkit.user-mode 标记）
	Runtime        string // 服务实际读取的运行时暂存目录
	State          string
	Bin            string
	SingBoxBin     string
	SingBoxVersion string
	UI             string
	Ruleset        string
	Downloads      string
	Subscriptions  string
	ActiveFile     string
	ConfigFile     string // 生效配置：真正的 sing-box JSON
	CustomizeFile  string
	GeositeCN      string // sing-box rule-set（.srs，二进制格式）
	GeoIPCN        string
}

func stateRoot(bundleRoot string) string {
	if v := os.Getenv("SBOXKIT_HOME"); v != "" {
		return v
	}
	if bundleRoot != "" {
		return filepath.Join(bundleRoot, UserModeStateDir)
	}
	return DefaultStateRoot
}

// Detect 依据环境变量与用户模式标记解析全部路径。
func Detect() Paths {
	bundleRoot := detectUserModeRoot(defaultUserModeProbe())
	state := stateRoot(bundleRoot)
	if bundleRoot != "" {
		return FromUserModeRoot(state, bundleRoot)
	}
	return FromRoot(state)
}

// FromRoot 以给定目录为状态根解析全部路径；供测试直接构造隔离的 Paths，
// 不必依赖 SBOXKIT_HOME 环境变量。生产代码一律用 Detect。
func FromRoot(s string) Paths {
	bin := filepath.Join(s, "bin")
	rs := filepath.Join(s, "ruleset")
	return Paths{
		Runtime:        RuntimeDir,
		State:          s,
		Bin:            bin,
		SingBoxBin:     filepath.Join(bin, "sing-box"),
		SingBoxVersion: filepath.Join(bin, "sing-box.version"),
		UI:             filepath.Join(s, "ui"),
		Ruleset:        rs,
		Downloads:      filepath.Join(s, "downloads"),
		Subscriptions:  filepath.Join(s, "subscriptions"),
		ActiveFile:     filepath.Join(s, "active"),
		ConfigFile:     filepath.Join(s, "config.json"),
		CustomizeFile:  filepath.Join(s, "customize.json"),
		GeositeCN:      filepath.Join(rs, "geosite-cn.srs"),
		GeoIPCN:        filepath.Join(rs, "geoip-cn.srs"),
	}
}

// EnsureStateDirs 创建所有运行期目录（幂等）。
func (p Paths) EnsureStateDirs() error {
	for _, d := range []string{p.State, p.Bin, p.UI, p.Ruleset, p.Downloads, p.Subscriptions} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// SubscriptionDir 某订阅的存储目录。
func (p Paths) SubscriptionDir(name string) string {
	return filepath.Join(p.Subscriptions, name)
}
