package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/webui"
)

func runUI(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit ui serve [--root DIR] [--addr ADDR]")
		return 0
	}
	if args[0] != "serve" {
		return fail(stderr, "unknown ui command: %s", args[0])
	}
	root, rest := parseRoot(args[1:])
	addr := valueFlag(rest, "--addr", "127.0.0.1:8790")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(stdout, "sboxkit ui listening on http://%s\n", addr)
	if err := webui.NewServer(paths.FromRoot(root)).ListenAndServe(ctx, addr); err != nil {
		return fail(stderr, "serve ui: %v", err)
	}
	return 0
}
