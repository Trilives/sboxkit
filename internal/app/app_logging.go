package app

import (
	"io"

	"github.com/Trilives/sboxkit/internal/applog"
	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

func runWithFileLog(args []string, stdout io.Writer, stderr io.Writer, run func(io.Writer) int) int {
	p := paths.FromRoot(rootFromCommandArgs(args))
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil || !cfg.EnableFileLog {
		return run(stderr)
	}
	writer, closeFn, err := applog.Open(p.LogDir, applog.Config{
		Enabled:  cfg.EnableFileLog,
		MaxBytes: applog.MaxBytes(cfg.LogMaxMB),
	}, stderr)
	if err != nil {
		return run(stderr)
	}
	applog.WriteHeader(writer, args)
	code := run(writer)
	applog.WriteFooter(writer, code)
	_ = closeFn()
	return code
}

func rootFromCommandArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	root, _ := parseRoot(args[1:])
	return root
}
