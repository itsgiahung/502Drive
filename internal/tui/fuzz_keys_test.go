package tui

import (
	"fmt"
	"math/rand"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Random keys and stray messages on every screen must never panic: a crash
// drops the user out of the app mid-task (one did, on the filter editor).
func TestRandomKeysNeverPanic(t *testing.T) {
	keys := []tea.KeyMsg{
		{Type: tea.KeyUp}, {Type: tea.KeyDown}, {Type: tea.KeyLeft}, {Type: tea.KeyRight},
		{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeySpace}, {Type: tea.KeyTab}, {Type: tea.KeyShiftTab},
		{Type: tea.KeyBackspace}, {Type: tea.KeyCtrlP}, {Type: tea.KeyCtrlS},
	}
	for _, r := range "1234567abcdefghijklmnopqrstuvwxyzABFJKPUEHRS/?[]+- " {
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	for seed := int64(1); seed <= 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		m := tabModel(t, 100, 30)
		var log []string
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("seed %d panicked after %v: %v", seed, log, r)
				}
			}()
			for i := 0; i < 400; i++ {
				var msg tea.Msg = keys[rng.Intn(len(keys))]
				if rng.Intn(12) == 0 {
					msg = struct{}{} // a blink or other stray message
				}
				log = append(log, fmt.Sprint(msg))
				if len(log) > 12 {
					log = log[1:]
				}
				next, _ := m.Update(msg)
				m = next.(Model)
				_ = m.View()
			}
		}()
	}
}
