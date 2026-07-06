package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"goaldl/pkg/serial"
)

// portPicker is the pre-session screen shown when a bare `goaldl` finds 0 or 2+
// USB serial ports (so auto-connect can't pick one). It re-polls on a 1s tick,
// auto-connects when the count drops to exactly one, and on Enter returns the
// highlighted port. q / ctrl+c cancel (chosen stays ""). It runs as its own tiny
// Bubble Tea program before the dashboard, so session construction stays in one
// place (cmdTUI) — the picker only resolves which port to hand it.
type portPicker struct {
	list   func() ([]string, error) // injectable for tests; nil → serial.AvailablePorts
	ports  []string
	cursor int
	err    error
	chosen string // set on Enter / auto-advance; "" means the user quit
	done   bool
}

type portTickMsg struct{}

func newPortPicker(list func() ([]string, error), initial []string) portPicker {
	if list == nil {
		list = serial.AvailablePorts
	}
	return portPicker{list: list, ports: initial}
}

func portTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return portTickMsg{} })
}

func (p portPicker) Init() tea.Cmd { return portTick() }

func (p portPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case portTickMsg:
		ports, err := p.list()
		p.err = err
		p.ports = ports
		if p.cursor >= len(ports) {
			p.cursor = max(0, len(ports)-1)
		}
		// Exactly one port now → unambiguous, connect (converges to the bare-goaldl
		// happy path once the extra device disappears).
		if len(ports) == 1 {
			p.chosen, p.done = ports[0], true
			return p, tea.Quit
		}
		return p, portTick()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			p.done = true // chosen stays "" — the user declined to connect
			return p, tea.Quit
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
		case "down", "j":
			if p.cursor < len(p.ports)-1 {
				p.cursor++
			}
		case "enter":
			if len(p.ports) > 0 {
				p.chosen, p.done = p.ports[p.cursor], true
				return p, tea.Quit
			}
		}
	}
	return p, nil
}

func (p portPicker) View() string {
	var b strings.Builder
	b.WriteString("\n  " + brandStyle.Render("GoALDL") + "\n\n")
	switch {
	case p.err != nil:
		fmt.Fprintf(&b, "  %s\n", beatBad.Render("port scan failed: "+p.err.Error()))
		b.WriteString("  " + dimStyle.Render("(retrying…)") + "\n\n")
	case len(p.ports) == 0:
		b.WriteString("  " + beatWarn.Render("No serial ports found — plug in the adapter") + " " + dimStyle.Render("(retrying…)") + "\n\n")
		b.WriteString("  " + dimStyle.Render("macOS needs Prolific's \"PL2303 Serial\" App Store driver for a PL2303 adapter.") + "\n\n")
	default:
		b.WriteString("  " + dimStyle.Render("Select a port  ([↑/↓] move · [enter] connect · [q] quit)") + "\n\n")
		for i, port := range p.ports {
			marker := "  "
			line := port
			if i == p.cursor {
				marker = "› "
				line = tabActive.Render(port)
			}
			fmt.Fprintf(&b, "  %s%s\n", marker, line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// runPortPicker runs the picker to completion and returns the chosen port, or ""
// if the user quit.
func runPortPicker(initial []string) string {
	final, err := tea.NewProgram(newPortPicker(nil, initial), tea.WithAltScreen()).Run()
	if err != nil {
		return ""
	}
	if fp, ok := final.(portPicker); ok {
		return fp.chosen
	}
	return ""
}

// stdinIsInteractive reports whether stdin is a terminal — the picker (and any
// interactive TUI) needs one. Stdlib char-device check (no x/term dependency).
func stdinIsInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
