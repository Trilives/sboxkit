// 生成器兼容性自检：用当前 sboxkit 的生成器为生效订阅重新生成配置，并以指定的内核与
// 规则集执行 `sing-box check`。内核 / 规则集更新在进程内调用；sboxkit 自更新时旧版本
// 经内部子命令 `compat-check` 调用新版本，结论以 compat.Report（JSON）交换。
package flows

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/converter"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
)

// Version 当前运行的 sboxkit 版本（由 main 设置），即进程内生成器的版本。
var Version = "dev"

// RunCompatCheck 内部子命令 `compat-check --kernel K --geosite G --geoip I --report R [--out O]`：
// 结论写入 R，兼容时把生成的配置写入 O（0600，含节点凭据）。写出报告即以 0 退出；
// 参数错误以 2 退出。
func RunCompatCheck(p paths.Paths, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("compat-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundle := compat.StateBundle(p)
	fs.StringVar(&bundle.Kernel, "kernel", bundle.Kernel, "sing-box binary to check with")
	fs.StringVar(&bundle.Geosite, "geosite", bundle.Geosite, "geosite-cn.srs to check with")
	fs.StringVar(&bundle.GeoIP, "geoip", bundle.GeoIP, "geoip-cn.srs to check with")
	reportPath := fs.String("report", "", "write the JSON report here")
	outPath := fs.String("out", "", "write the regenerated config here when compatible")
	if err := fs.Parse(args); err != nil || *reportPath == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: sboxkit compat-check --kernel K --geosite G --geoip I --report R [--out O]")
		return 2
	}
	report, cfg := checkGenerator(p, bundle)
	if cfg != nil && *outPath != "" {
		if err := os.WriteFile(*outPath, cfg, 0o600); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := compat.WriteReport(*reportPath, report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// checkGenerator 用当前生成器为生效订阅重新生成配置并以 b 校验；兼容时一并返回配置。
// 没有生效订阅时视为兼容（无可重新生成的内容）；无法重新生成时记入 Unverified。
func checkGenerator(p paths.Paths, b compat.Bundle) (compat.Report, []byte) {
	r := compat.Report{Generator: Version, MinKernel: converter.MinSingBoxVersion, Kernel: compat.KernelVersion(b.Kernel)}
	active := subscription.GetActive(p)
	if active == nil {
		r.Compatible = true
		return r, nil
	}
	r.Subscription = active.Name
	if ok, known := compat.AtLeast(r.Kernel, r.MinKernel); known && !ok {
		r.Error = fmt.Sprintf(i18n.T("该生成器要求 sing-box ≥ %s"), r.MinKernel)
		r.Hint = i18n.T("请同时更新 sing-box 内核，或继续使用原 sboxkit 版本。")
		return r, nil
	}
	cfg, err := subscription.Generate(p, active.Name)
	if err != nil {
		r.Unverified = fmt.Sprintf(i18n.T("无法为生效订阅「%s」重新生成配置，未能校验生成器：%v"), active.Name, err)
		return r, nil
	}
	var rejected *compat.Error
	switch err := compat.Check(p, cfg, b); {
	case err == nil:
		r.Compatible = true
		return r, cfg
	case errors.As(err, &rejected):
		r.Error, r.Hint = rejected.Output, rejected.Hint()
	case errors.Is(err, compat.ErrKernelMissing):
		r.Unverified = i18n.T("尚无 sing-box 内核，未能校验生成器。")
	default:
		r.Error = err.Error()
	}
	return r, nil
}
