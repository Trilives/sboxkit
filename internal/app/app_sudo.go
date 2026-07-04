package app

import (
	ui "github.com/Trilives/sboxkit/internal/tui"
	"github.com/Trilives/sboxkit/internal/system"
)

func sudoPasswordPrompt(prompt string) (string, error) {
	return ui.Password(prompt)
}

func newSudoRunner() system.OSRunner {
	return system.OSRunner{UseSudo: true, PromptPassword: sudoPasswordPrompt}
}
