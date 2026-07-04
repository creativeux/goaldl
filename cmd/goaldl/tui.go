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

// cmdTUI launches the interactive terminal dashboard: one entry point that
// navigates between the sensor table, the BLM grid, and a raw frame view,
// driven by either a live ECM (-p) or a replayed capture file.
//
//	goaldl tui -p /dev/cu.usbserial-10
//	goaldl tui drive_4800.raw [-speed 2]
func cmdTUI() {
	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	portName := fs.String("p", "", "Live: serial port to read from (omit to replay a file)")
	baudRate := fs.Int("b", 4800, "UART sampling baud rate")
	ecmPart := fs.String("e", defaultECM, "ECM part number")
	promID := fs.Int("prom", 6291, "Expected PROM ID for the sync indicator (0 to disable)")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	minSamples := fs.Int("min", blm.DefaultMinSamples, "BLM: samples before a cell is trusted")
	speed := fs.Float64("speed", 1.0, "Replay only: playback speed (1=real time, 0=as fast as possible)")
	fs.Parse(os.Args[2:])

	cfg := decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert}
	registry := ecm.NewRegistry()
	if _, ok := registry.GetDefinition(*ecmPart); !ok {
		fmt.Fprintf(os.Stderr, "Unknown ECM: %s\n", *ecmPart)
		os.Exit(1)
	}

	var provider stream.Provider
	var source string
	if *portName != "" {
		provider = &stream.SerialProvider{Port: *portName, Baud: *baudRate, Config: cfg}
		source = "live " + *portName
	} else {
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Usage: goaldl tui -p <port>   |   goaldl tui <capture.raw>")
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
		source = "replay " + inName
	}

	// Run the provider in the background, delivering frames over a channel. The
	// emit blocks on the channel, so the provider is naturally paced by the UI.
	ctx, cancel := context.WithCancel(context.Background())
	frames := make(chan stream.FrameEvent)
	go func() {
		provider.Run(ctx, func(ev stream.FrameEvent) {
			select {
			case frames <- ev:
			case <-ctx.Done():
			}
		})
		close(frames)
	}()

	m := tuiModel{
		registry:   registry,
		ecmPart:    *ecmPart,
		promID:     *promID,
		minSamples: *minSamples,
		source:     source,
		frames:     frames,
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

// frameMsg carries one decoded frame into the update loop; providerDoneMsg
// signals the stream ended (replay finished or port closed).
type (
	frameMsg        stream.FrameEvent
	providerDoneMsg struct{}
)

type tuiModel struct {
	// config / wiring
	registry   *ecm.Registry
	ecmPart    string
	promID     int
	minSamples int
	source     string
	frames     <-chan stream.FrameEvent
	cancel     context.CancelFunc

	// state
	width, height int
	active        view
	latest        stream.FrameEvent
	hasFrame      bool
	grid          *blm.Grid
	frameCount    int
	done          bool
}

// waitForFrame blocks on the frame channel and delivers the next event as a
// message. Re-issued after each frame to keep the stream flowing.
func (m tuiModel) waitForFrame() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.frames
		if !ok {
			return providerDoneMsg{}
		}
		return frameMsg(ev)
	}
}

func (m tuiModel) Init() tea.Cmd { return m.waitForFrame() }

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

	case frameMsg:
		ev := stream.FrameEvent(msg)
		m.latest = ev
		m.hasFrame = true
		m.frameCount++
		if ft := ecm.FuelTrimSample(ev.Frame.Data); ft.Recordable() {
			m.grid.Add(ft.RPM, ft.MapKPa, ft.BLM)
		}
		return m, m.waitForFrame()

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
	right := dimStyle.Render(m.source)
	header := lipgloss.JoinHorizontal(lipgloss.Top, rendered...) + "   " + right

	var body string
	switch {
	case !m.hasFrame:
		body = dimStyle.Render("\n  waiting for frames…")
	case m.active == viewSensors:
		body = stream.SensorTable(m.latest, m.registry, m.ecmPart)
	case m.active == viewBLM:
		body = stream.BLMBody(m.grid, m.latest, m.minSamples)
	case m.active == viewRaw:
		body = m.rawView()
	}

	status := fmt.Sprintf("frame %d   t=%.1fs   %s",
		m.latest.Index, m.latest.Elapsed.Seconds(), m.promMark())
	if m.done {
		status += "   " + dimStyle.Render("(stream ended)")
	}
	keys := dimStyle.Render("1-3/tab switch · q quit")
	footer := status + "   " + keys

	return header + "\n\n" + body + "\n\n" + footer
}

func (m tuiModel) rawView() string {
	f := m.latest.Frame.Data
	var hex strings.Builder
	for i, b := range f {
		if i > 0 && i%10 == 0 {
			hex.WriteByte('\n')
			hex.WriteString("           ")
		}
		fmt.Fprintf(&hex, " %02X", b)
	}
	return fmt.Sprintf("  offset %d\n  bytes:  %s\n\n  %s",
		m.latest.Frame.ByteOffset, strings.TrimSpace(hex.String()), m.promMark())
}

func (m tuiModel) promMark() string {
	f := m.latest.Frame.Data
	if m.promID == 0 || len(f) < 3 {
		return ""
	}
	if int(f[1])<<8|int(f[2]) == m.promID {
		return "PROM ✓"
	}
	return "PROM ✗"
}
