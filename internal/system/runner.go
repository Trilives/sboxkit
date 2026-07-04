package system

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type OSRunner struct {
	UseSudo        bool
	PromptPassword func(prompt string) (string, error)
}

func (r OSRunner) Run(ctx context.Context, name string, args ...string) error {
	if r.UseSudo && os.Geteuid() != 0 {
		if r.PromptPassword != nil {
			password, err := r.PromptPassword("sudo password: ")
			if err != nil {
				return fmt.Errorf("prompt sudo password: %w", err)
			}
			args = append([]string{"-S", "-p", "", name}, args...)
			cmd := exec.CommandContext(ctx, "sudo", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			stdin, err := cmd.StdinPipe()
			if err != nil {
				return fmt.Errorf("run sudo: %w", err)
			}
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("run sudo: %w", err)
			}
			_, _ = io.WriteString(stdin, password+"\n")
			_ = stdin.Close()
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("run sudo: %w", err)
			}
			return nil
		}
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
