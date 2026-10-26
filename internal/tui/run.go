package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI and blocks until the user quits. The alternate screen and
// the terminal state are restored by Bubble Tea on exit; a panic in the model
// still kills the program so the terminal is not left in raw mode.
func Run(ctx context.Context, acts Actions, opt Options) (err error) {
	m := New(acts, opt)
	m.ctx = ctx
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	defer func() {
		if r := recover(); r != nil {
			p.Kill()
			err = fmt.Errorf("the menu stopped unexpectedly: %v", r)
		}
	}()
	if _, runErr := p.Run(); runErr != nil {
		return runErr
	}
	return nil
}
