package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type OSRunner struct {
	UseSudo bool
}

func (r OSRunner) Run(ctx context.Context, name string, args ...string) error {
	if r.UseSudo && os.Geteuid() != 0 {
		args = append([]string{name}, args...)
		name = "sudo"
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}
