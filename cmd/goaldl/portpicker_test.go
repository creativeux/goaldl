package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestPortPicker: the pre-session picker lists 2+ ports and moves a clamped
// cursor; Enter returns the highlighted port; a tick re-polls and auto-connects
// when the count drops to exactly one; 0 ports show the retry + driver hint; q
// returns "" (the user declined — exit 0, not an error).
func TestPortPicker(t *testing.T) {
	twoPorts := []string{"/dev/cu.usbserial-10", "/dev/cu.Bluetooth"}

	// Render + cursor movement (clamped at both ends).
	p := newPortPicker(func() ([]string, error) { return twoPorts, nil }, twoPorts)
	if v := p.View(); !strings.Contains(v, "Select a port") || !strings.Contains(v, twoPorts[0]) || !strings.Contains(v, twoPorts[1]) {
		t.Errorf("2-port view missing title or a port:\n%s", v)
	}
	up, _ := p.Update(tea.KeyMsg{Type: tea.KeyUp})
	if up.(portPicker).cursor != 0 {
		t.Errorf("cursor clamps at 0 going up, got %d", up.(portPicker).cursor)
	}
	dn, _ := p.Update(tea.KeyMsg{Type: tea.KeyDown})
	dn, _ = dn.(portPicker).Update(tea.KeyMsg{Type: tea.KeyDown}) // past the end
	if dn.(portPicker).cursor != 1 {
		t.Errorf("cursor clamps at last index, got %d", dn.(portPicker).cursor)
	}

	// Enter returns the highlighted port.
	sel, cmd := dn.(portPicker).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := sel.(portPicker); got.chosen != twoPorts[1] || !got.done {
		t.Errorf("Enter chose %q (done=%v), want %q", got.chosen, got.done, twoPorts[1])
	}
	if cmd == nil {
		t.Error("Enter should quit the picker")
	}

	// A tick that now reports exactly one port auto-connects to it.
	single := newPortPicker(func() ([]string, error) { return twoPorts[:1], nil }, twoPorts)
	adv, cmd := single.Update(portTickMsg{})
	if got := adv.(portPicker); got.chosen != twoPorts[0] || !got.done {
		t.Errorf("drop-to-one tick chose %q (done=%v), want auto-connect to %q", got.chosen, got.done, twoPorts[0])
	}
	if cmd == nil {
		t.Error("auto-connect should quit the picker")
	}

	// Zero ports: retry + PL2303 driver hint; a tick keeps polling (no quit).
	empty := newPortPicker(func() ([]string, error) { return nil, nil }, nil)
	if v := empty.View(); !strings.Contains(v, "No serial ports found") || !strings.Contains(v, "PL2303") {
		t.Errorf("empty view missing retry or driver hint:\n%s", v)
	}
	stay, cmd := empty.Update(portTickMsg{})
	if stay.(portPicker).done || cmd == nil {
		t.Error("zero-port tick should keep polling (not done), and schedule the next tick")
	}

	// A scan error is surfaced and polling continues.
	bad := newPortPicker(func() ([]string, error) { return nil, errors.New("boom") }, nil)
	e, _ := bad.Update(portTickMsg{})
	if em := e.(portPicker); em.err == nil || em.done {
		t.Error("a scan error should be recorded and not end the picker")
	}
	if !strings.Contains(e.(portPicker).View(), "port scan failed") {
		t.Error("error view should say the scan failed")
	}

	// q declines: done, chosen "" (caller exits 0).
	q, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if got := q.(portPicker); !got.done || got.chosen != "" {
		t.Errorf("q gave chosen=%q done=%v, want a clean decline", got.chosen, got.done)
	}
	if cmd == nil {
		t.Error("q should quit the picker")
	}
}
