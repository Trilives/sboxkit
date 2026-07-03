package updater

import (
	"context"
	"io"
	"os/exec"
)

type SystemdService struct{}

func (SystemdService) Stop(ctx context.Context) error {
	return exec.CommandContext(ctx, "systemctl", "stop", "sboxkit.service").Run()
}

func (SystemdService) Start(ctx context.Context) error {
	return exec.CommandContext(ctx, "systemctl", "restart", "sboxkit.service").Run()
}

type ExecVerifier struct{}

func (ExecVerifier) Verify(ctx context.Context, path string, args ...string) error {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
