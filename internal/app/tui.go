package app

import (
	"fmt"
	"io"
	"os"

	ui "github.com/Trilives/sboxkit/internal/tui"
)

type tuiAction func(*tuiSession) bool

type tuiItem struct {
	Label  string
	Detail string
	Action tuiAction
}

type tuiSession struct {
	stdout   io.Writer
	stderr   io.Writer
	selectF  func(string, []string, ui.SelectOpts) (int, error)
	askF     func(string, ui.AskOpts) (string, error)
	confirmF func(string, bool) (bool, error)
	pauseF   func(string)
}

func newTUISession(stdout io.Writer, stderr io.Writer) *tuiSession {
	return &tuiSession{
		stdout:   stdout,
		stderr:   stderr,
		selectF:  ui.Select,
		askF:     ui.Ask,
		confirmF: ui.Confirm,
		pauseF:   ui.Pause,
	}
}

func runTTYInteractive(stderr io.Writer) (int, bool) {
	if !ui.UseTUI() {
		return 0, false
	}
	return newTUISession(os.Stdout, stderr).run(), true
}

func (s *tuiSession) run() int {
	for {
		idx, ok := s.selectMenu("sboxkit", mainTUIItems())
		if !ok {
			return 0
		}
		if mainTUIItems()[idx].Action(s) {
			return 0
		}
	}
}

func submenu(title string, items func() []tuiItem) tuiAction {
	return func(s *tuiSession) bool {
		idx := 0
		for {
			next, ok := s.selectMenu(title, items(), idx)
			if !ok {
				return false
			}
			idx = next
			if items()[next].Action(s) {
				return true
			}
		}
	}
}

func commandAction(title string, run func(*tuiSession) int) tuiAction {
	return func(s *tuiSession) bool {
		fmt.Fprintf(s.stdout, "\n== %s ==\n\n", title)
		code := run(s)
		if code != 0 {
			fmt.Fprintf(s.stdout, "\nCommand exited with status %d.\n", code)
		}
		s.wait()
		return false
	}
}

func promptCommand(title string, build func(*tuiSession) ([]string, bool), run func([]string, io.Writer, io.Writer) int) tuiAction {
	return commandAction(title, func(s *tuiSession) int {
		args, ok := build(s)
		if !ok {
			fmt.Fprintln(s.stdout, "Cancelled.")
			return 0
		}
		fmt.Fprintln(s.stdout)
		return run(args, s.stdout, s.stderr)
	})
}

func configSetAction(key string, value string) tuiAction {
	return commandAction("Set "+key, func(s *tuiSession) int {
		return runConfig([]string{"set", "--key", key, "--value", value}, s.stdout, s.stderr)
	})
}

func (s *tuiSession) selectMenu(title string, items []tuiItem, initial ...int) (int, bool) {
	if len(items) == 0 {
		return 0, false
	}
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.Label
	}
	idx := 0
	if len(initial) > 0 {
		idx = initial[0]
	}
	selected, err := s.selectF(title, labels, ui.SelectOpts{BackLabel: "back", Initial: idx})
	if err != nil {
		return 0, false
	}
	return selected, true
}

func (s *tuiSession) promptRequired(label string) (string, bool) {
	for {
		value := s.promptDefault(label, "")
		if value != "" {
			return value, true
		}
		if !s.confirm("Leave this prompt?", false) {
			continue
		}
		return "", false
	}
}

func (s *tuiSession) promptDefault(label string, fallback string) string {
	value, err := s.askF(label, ui.AskOpts{Default: fallback, AllowEmpty: fallback == ""})
	if err != nil {
		return fallback
	}
	return value
}

func (s *tuiSession) confirm(label string, fallback bool) bool {
	value, err := s.confirmF(label, fallback)
	if err != nil {
		return fallback
	}
	return value
}

func (s *tuiSession) wait() {
	s.pauseF("Press Enter to return to the menu...")
}

func (s *tuiSession) confirmServiceTrafficRisk(action string) bool {
	fmt.Fprintln(s.stdout, serviceTrafficWarning())
	return s.confirm("Continue to "+action+"?", false)
}
