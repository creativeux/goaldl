package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/stream"
)

// cmdTUI launches the interactive dashboard — the default face of goaldl. It
// navigates between the sensor table, the BLM grid, the flag-data and
// error-code views, and a scrolling raw-byte history, driven by a
// stream.Session over either a live ECM (-p) or a replayed capture.
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
	tps0 := fs.Float64("tps0", ecm.DefaultTPS0, "TPS calibration: volts at 0% throttle")
	tps100 := fs.Float64("tps100", ecm.DefaultTPS100, "TPS calibration: volts at 100% throttle")
	speed := fs.Float64("speed", 1.0, "Replay only: playback speed (1=real time, 0=as fast as possible)")
	fs.Parse(args)

	cfg := decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert}
	registry := ecm.NewRegistry()
	def, ok := registry.GetDefinition(*ecmPart)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown ECM: %s\n", *ecmPart)
		os.Exit(1)
	}
	def = calibratedDef(def, *tps0, *tps100)

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
		def:        def,
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

// calibratedDef applies the TPS calibration flags to a definition copy,
// falling back to the defaults (with a warning) on a degenerate range.
func calibratedDef(def *ecm.Definition, tps0, tps100 float64) *ecm.Definition {
	if tps100 <= tps0 {
		fmt.Fprintf(os.Stderr, "Invalid TPS calibration (%.2f..%.2fV); using defaults %.2f..%.2fV\n",
			tps0, tps100, ecm.DefaultTPS0, ecm.DefaultTPS100)
		return def
	}
	return def.WithTPSCalibration(tps0, tps100)
}

type view int

const (
	viewSensors view = iota
	viewBLM
	viewFlags
	viewCodes
	viewRaw
	viewCount
)

// rawHistoryCap bounds the raw-frame ring; more than the widest terminal can
// show (the view itself caps at 14 columns, WinALDL-style).
const rawHistoryCap = 64

// snapshotMsg carries one processed frame into the update loop; providerDoneMsg
// signals the stream ended (replay finished or port closed).
type (
	snapshotMsg     stream.Snapshot
	providerDoneMsg struct{}
)

type tuiModel struct {
	// config / wiring
	def        *ecm.Definition
	minSamples int
	source     string
	snaps      <-chan stream.Snapshot
	cancel     context.CancelFunc

	// state
	width, height int
	active        view
	latest        stream.Snapshot // every frame, drives the raw view + heartbeat
	lastGood      stream.Snapshot // latest ParseOK frame, drives all decoded views
	hasFrame      bool
	hasGood       bool
	history       [][]byte // raw frames, newest first, capped at rawHistoryCap
	okCount       int
	badCount      int
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
			m.active = viewFlags
		case "4":
			m.active = viewCodes
		case "5":
			m.active = viewRaw
		case "tab", "right", "l":
			m.active = (m.active + 1) % viewCount
		case "shift+tab", "left", "h":
			m.active = (m.active + viewCount - 1) % viewCount
		}

	case snapshotMsg:
		s := stream.Snapshot(msg)
		m.latest = s
		m.hasFrame = true
		m.frameCount++
		// The raw history takes every frame — the raw view is never gated
		// (WinALDL behavior: a bad sample still updates the RAW tab). The
		// decoded views render from lastGood instead.
		frame := make([]byte, len(s.Frame.Data))
		copy(frame, s.Frame.Data)
		m.history = append([][]byte{frame}, m.history...)
		if len(m.history) > rawHistoryCap {
			m.history = m.history[:rawHistoryCap]
		}
		if s.ParseOK {
			m.lastGood = s
			m.hasGood = true
			m.okCount++
		} else {
			m.badCount++
		}
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
	beatOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	beatBad     = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
)

func (m tuiModel) View() string {
	tabs := []string{"1 Sensors", "2 BLM grid", "3 Flags", "4 Codes", "5 Raw"}
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
	case m.active == viewRaw:
		body = m.rawView()
	case !m.hasGood:
		body = dimStyle.Render("\n  waiting for a parseable frame… (see 5 Raw for the byte stream)")
	case m.active == viewSensors:
		body = stream.SensorTable(m.lastGood.FrameEvent, m.def)
	case m.active == viewBLM:
		body = stream.BLMBody(m.grid, m.lastGood.FrameEvent, m.minSamples)
	case m.active == viewFlags:
		body = stream.FlagsBody(m.lastGood.Flags)
	case m.active == viewCodes:
		body = stream.CodesBody(m.lastGood.Codes)
	}

	status := fmt.Sprintf("frame %d   t=%.1fs   %s   %s %d ok / %d bad",
		m.latest.Index, m.latest.Elapsed.Seconds(), promMark(m.latest.PROMOK),
		m.heartbeat(), m.okCount, m.badCount)
	if m.done {
		status += "   " + dimStyle.Render("(stream ended)")
	}
	footer := status + "   " + dimStyle.Render("1-5/tab switch · q quit")

	return header + "\n\n" + body + "\n\n" + footer
}

// heartbeat is the per-frame data-quality tick: green when the latest frame
// parsed and PROM-matched, red otherwise (WinALDL's flashing indicator).
func (m tuiModel) heartbeat() string {
	if m.latest.ParseOK && m.latest.PROMOK {
		return beatOK.Render("●")
	}
	return beatBad.Render("●")
}

func (m tuiModel) rawView() string {
	head := fmt.Sprintf("  offset %d   %s\n\n", m.latest.Frame.ByteOffset, promMark(m.latest.PROMOK))
	return head + stream.RawHistory(m.def.ByteLabels, m.history, m.width)
}

func promMark(ok bool) string {
	if ok {
		return "PROM ✓"
	}
	return "PROM ✗"
}
