package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/stream"
)

// cmdTUI launches the interactive dashboard — the default face of goaldl. It
// navigates between the sensor table, the BLM grid, and a raw frame view,
// driven by a stream.Session over either a live ECM (-p) or a replayed capture.
//
//	goaldl -p /dev/cu.usbserial-10
//	goaldl drive_4800.raw [-speed 2]
func cmdTUI(args []string) {
	fs := flag.NewFlagSet("goaldl", flag.ExitOnError)
	portName := fs.String("p", "", "Live: serial port to read from (omit to replay a file)")
	baudRate := fs.Int("b", 4800, "UART sampling baud rate")
	ecmPart := fs.String("e", defaultECM, "ECM part number")
	promID := fs.Int("prom", 6291, "Expected PROM ID for the sync indicator (0 to disable)")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	minSamples := fs.Int("min", blm.DefaultMinSamples, "BLM: samples before a cell is trusted")
	speed := fs.Float64("speed", 1.0, "Replay only: playback speed (1=real time, 0=as fast as possible)")
	fs.Parse(args)

	cfg := decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert}
	registry := ecm.NewRegistry()
	if _, ok := registry.GetDefinition(*ecmPart); !ok {
		fmt.Fprintf(os.Stderr, "Unknown ECM: %s\n", *ecmPart)
		os.Exit(1)
	}

	var provider stream.Provider
	if *portName != "" {
		provider = &stream.SerialProvider{Port: *portName, Baud: *baudRate, Config: cfg}
	} else {
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "No source. Give a port or a capture file:")
			fmt.Fprintln(os.Stderr, "  goaldl -p /dev/cu.usbserial-10")
			fmt.Fprintln(os.Stderr, "  goaldl drive_4800.raw")
			fmt.Fprintln(os.Stderr, "\nSee 'goaldl help' for the scripting commands (ports, record, decode, blm, …).")
			os.Exit(1)
		}
		inName := fs.Arg(0)
		fs.Parse(fs.Args()[1:]) // allow flags after the filename
		data, err := os.ReadFile(inName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", inName, err)
			os.Exit(1)
		}
		provider = &stream.ReplayProvider{Data: data, Config: cfg, Speed: *speed}
	}

	session := stream.NewSession(provider, registry, *ecmPart, *promID)

	// Run the session in the background, delivering snapshots over a channel.
	// The emit blocks on the channel, so the session is paced by the UI.
	ctx, cancel := context.WithCancel(context.Background())
	// Small buffer so a briefly-stalled terminal (slow SSH, flow control) can't
	// block the session goroutine from draining the UART and dropping bytes.
	snaps := make(chan stream.Snapshot, 8)
	go func() {
		session.Run(ctx, func(s stream.Snapshot) {
			select {
			case snaps <- s:
			case <-ctx.Done():
			}
		})
		close(snaps)
	}()

	m := tuiModel{
		registry:   registry,
		ecmPart:    *ecmPart,
		minSamples: *minSamples,
		source:     session.Name(),
		snaps:      snaps,
		cancel:     cancel,
		grid:       blm.NewDefault(),
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
	cancel()
}

type view int

const (
	viewSensors view = iota
	viewBLM
	viewRaw
)

// snapshotMsg carries one processed frame into the update loop; providerDoneMsg
// signals the stream ended (replay finished or port closed).
type (
	snapshotMsg     stream.Snapshot
	providerDoneMsg struct{}
)

type tuiModel struct {
	// config / wiring
	registry   *ecm.Registry
	ecmPart    string
	minSamples int
	source     string
	snaps      <-chan stream.Snapshot
	cancel     context.CancelFunc

	// state
	width, height int
	active        view
	latest        stream.Snapshot
	hasFrame      bool
	grid          *blm.Grid
	frameCount    int
	done          bool
}

// waitForSnapshot blocks on the snapshot channel and delivers the next as a
// message. Re-issued after each snapshot to keep the stream flowing.
func (m tuiModel) waitForSnapshot() tea.Cmd {
	return func() tea.Msg {
		s, ok := <-m.snaps
		if !ok {
			return providerDoneMsg{}
		}
		return snapshotMsg(s)
	}
}

func (m tuiModel) Init() tea.Cmd { return m.waitForSnapshot() }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.cancel()
			return m, tea.Quit
		case "1":
			m.active = viewSensors
		case "2":
			m.active = viewBLM
		case "3":
			m.active = viewRaw
		case "tab", "right", "l":
			m.active = (m.active + 1) % 3
		case "shift+tab", "left", "h":
			m.active = (m.active + 2) % 3
		}

	case snapshotMsg:
		s := stream.Snapshot(msg)
		m.latest = s
		m.hasFrame = true
		m.frameCount++
		if s.FuelTrim.Recordable() {
			m.grid.Add(s.FuelTrim.RPM, s.FuelTrim.MapKPa, s.FuelTrim.BLM)
		}
		return m, m.waitForSnapshot()

	case providerDoneMsg:
		m.done = true
	}
	return m, nil
}

var (
	tabActive   = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	dimStyle    = lipgloss.NewStyle().Faint(true)
)

func (m tuiModel) View() string {
	tabs := []string{"1 Sensors", "2 BLM grid", "3 Raw"}
	rendered := make([]string, len(tabs))
	for i, t := range tabs {
		if view(i) == m.active {
			rendered[i] = tabActive.Render(t)
		} else {
			rendered[i] = tabInactive.Render(t)
		}
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top, rendered...) + "   " + dimStyle.Render(m.source)

	var body string
	switch {
	case !m.hasFrame:
		body = dimStyle.Render("\n  waiting for frames…")
	case m.active == viewSensors:
		body = stream.SensorTable(m.latest.FrameEvent, m.registry, m.ecmPart)
	case m.active == viewBLM:
		body = stream.BLMBody(m.grid, m.latest.FrameEvent, m.minSamples)
	case m.active == viewRaw:
		body = m.rawView()
	}

	status := fmt.Sprintf("frame %d   t=%.1fs   %s",
		m.latest.Index, m.latest.Elapsed.Seconds(), promMark(m.latest.PROMOK))
	if m.done {
		status += "   " + dimStyle.Render("(stream ended)")
	}
	footer := status + "   " + dimStyle.Render("1-3/tab switch · q quit")

	return header + "\n\n" + body + "\n\n" + footer
}

func (m tuiModel) rawView() string {
	f := m.latest.Frame.Data
	var hex strings.Builder
	for i, b := range f {
		if i > 0 && i%10 == 0 {
			hex.WriteString("\n           ")
		}
		fmt.Fprintf(&hex, " %02X", b)
	}
	return fmt.Sprintf("  offset %d\n  bytes:  %s\n\n  %s",
		m.latest.Frame.ByteOffset, strings.TrimSpace(hex.String()), promMark(m.latest.PROMOK))
}

func promMark(ok bool) string {
	if ok {
		return "PROM ✓"
	}
	return "PROM ✗"
}
