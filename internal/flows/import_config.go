package flows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/configfile"
	"github.com/Trilives/sboxkit/internal/deploy"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
	"github.com/Trilives/sboxkit/internal/txn"
)

func importConfigFlow(p paths.Paths) error {
	path, err := tui.Ask(i18n.T("YAML 配置文件路径"), tui.AskOpts{AllowEmpty: false})
	if err != nil {
		return err
	}
	raw, err := loadImportConfig(p, path)
	if err != nil {
		return err
	}
	sync := false
	if sysd.IsInstalled(sysd.DefaultName) {
		if sync, err = tui.Confirm(i18n.T("服务已安装，立即同步并重启以使用该配置？"), true); err != nil {
			return err
		}
	}
	if err := applyImportConfig(p, raw, sync); err != nil {
		return err
	}
	execx.Ok(i18n.T("已导入 YAML 配置文件，并设为当前生效配置。"))
	return nil
}

// resolveLocalPath 展开 ~/ 前缀、转绝对路径、校验存在且不是目录；供「本地文件
// 覆盖」与「添加订阅（本地 YAML 文件）」两条路径共用。
func resolveLocalPath(sourcePath string) (string, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return "", fmt.Errorf("%s", i18n.T("本地文件路径不能为空"))
	}
	if strings.HasPrefix(sourcePath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			sourcePath = filepath.Join(home, strings.TrimPrefix(sourcePath, "~/"))
		}
	}
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(absSource)
	if err != nil {
		return "", fmt.Errorf(i18n.T("读取本地文件: %w"), err)
	}
	if st.IsDir() {
		return "", fmt.Errorf(i18n.T("请输入文件路径，而不是目录: %s"), absSource)
	}
	return absSource, nil
}

// loadImportConfig 读取并校验待导入的配置：先解析，再用当前内核与规则集 check；
// 不写盘。
func loadImportConfig(p paths.Paths, sourcePath string) ([]byte, error) {
	absSource, err := resolveLocalPath(sourcePath)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(absSource)
	if err != nil {
		return nil, err
	}
	if _, err := configfile.Parse(raw); err != nil {
		return nil, fmt.Errorf(i18n.T("解析 YAML 配置文件: %w"), err)
	}
	execx.Info(i18n.T("用当前内核校验导入的配置…"))
	err = compat.Check(p, raw, compat.StateBundle(p))
	if errors.Is(err, compat.ErrKernelMissing) {
		execx.Warn(i18n.T("尚无 sing-box 内核，暂无法校验配置；注册服务前会再次校验。"))
		return raw, nil
	}
	if err != nil {
		return nil, fmt.Errorf(i18n.T("导入的配置未通过校验，原配置与服务均保持不变：%w"), err)
	}
	return raw, nil
}

// applyImportConfig 把已校验的配置设为生效配置并清除 active 指针；sync 时同步服务，
// 失败则连同生效配置与 active 指针一起回退。
func applyImportConfig(p paths.Paths, raw []byte, sync bool) error {
	return deploy.Apply(p, deploy.Change{
		Name:     i18n.T("导入配置文件"),
		Snapshot: []string{p.ConfigFile, p.ActiveFile},
		Sync:     sync,
		Commit: func(*txn.Transaction) error {
			if err := writeFileAtomic(p.ConfigFile, raw, 0o644); err != nil {
				return err
			}
			if err := os.Remove(p.ActiveFile); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		},
	})
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".import-config.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
