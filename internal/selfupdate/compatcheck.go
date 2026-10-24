package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

const compatCheckTimeout = 2 * time.Minute

// ErrNoCompatCheck 目标版本没有 compat-check 子命令（早于该机制），无法验证其生成器。
var ErrNoCompatCheck = errors.New("target sboxkit does not support compat-check")

// CompatCheck 让该版本 sboxkit 用自身生成器为生效订阅重新生成配置，并以 b 中的内核与
// 规则集校验；兼容时一并返回生成的配置（没有生效订阅时为 nil）。子进程沿用本进程的
// 状态目录、模式与界面语言。
func (t *Prepared) CompatCheck(p paths.Paths, b compat.Bundle) (compat.Report, []byte, error) {
	dir, err := os.MkdirTemp("", "sboxkit-compat-*")
	if err != nil {
		return compat.Report{}, nil, err
	}
	defer os.RemoveAll(dir)
	reportPath, outPath := filepath.Join(dir, "report.json"), filepath.Join(dir, "config.json")
	ctx, cancel := context.WithTimeout(context.Background(), compatCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.Bin, "compat-check",
		"--kernel", b.Kernel, "--geosite", b.Geosite, "--geoip", b.GeoIP,
		"--report", reportPath, "--out", outPath)
	cmd.Env = append(os.Environ(),
		"SBOXKIT_HOME="+p.State, "SBOXKIT_USER_MODE="+userModeValue(p), "SBOXKIT_LANG="+string(i18n.Current()))
	output, runErr := cmd.CombinedOutput()
	report, err := compat.ReadReport(reportPath)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if runErr != nil && detail == "" {
			detail = runErr.Error()
		}
		return compat.Report{}, nil, fmt.Errorf("%w: %s", ErrNoCompatCheck, detail)
	}
	if !report.Compatible || report.Subscription == "" {
		return report, nil, nil
	}
	cfg, err := os.ReadFile(outPath)
	if err != nil {
		return report, nil, fmt.Errorf(i18n.T("读取新版 sboxkit 生成的配置：%w"), err)
	}
	return report, cfg, nil
}

func userModeValue(p paths.Paths) string {
	if p.UserMode {
		return "1"
	}
	return "0"
}
