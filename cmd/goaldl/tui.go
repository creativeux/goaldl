package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/stream"
)

// errNoTUISource signals that neither a port nor a capture file was given, so
// cmdTUI can print its detailed source-selection help.
var errNoTUISource = errors.New("no source")

// tuiFlags is the fully-resolved dashboard configuration after the two-stage
// parse (flags may trail the capture filename). It is plain data so the
// parse-and-resolve step can be unit-tested without launching the TUI.
type tuiFlags struct {
	cfg        decoder.Config
	registry   *ecm.Registry
	def        *ecm.Definition // TPS-calibrated copy
	ecmPart    string
	promID     int
	minSamples int
	portName   string
	inName     string // capture file (empty for live)
	speed      float64
}

// resolveTUIFlags parses the dashboard flags, honouring flags that trail the
// capture filename (`goaldl drive.raw -tps0 0.5 -e <part>`). Every flag value
// is read only after the trailing re-parse, so post-filename flags are not
// silently dropped. Returns errNoTUISource when no port/file is given.
func resolveTUIFlags(args []string) (*tuiFlags, error) {
	fs := flag.NewFlagSet("goaldl", flag.ContinueOnError)
	portName := fs.String("p", "", "Live: serial port to read from (omit to replay a file)")
	baudRate := fs.Int("b", 4800, "UART sampling baud rate")
	ecmPart := fs.String("e", defaultECM, "ECM part number")
	promID := fs.Int("prom", 6291, "Expected PROM ID for the sync indicator (0 to disable)")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	minSamples := fs.Int("min", blm.DefaultMinSamples, "BLM: samples before a cell is trusted")
	tps0 := fs.Float64("tps0", ecm.DefaultTPS0, "TPS calibration: volts at 0% throttle")
	tps100 := fs.Float64("tps100", ecm.DefaultTPS100, "TPS calibration: volts at 100% throttle")
	speed := fs.Float64("speed", 1.0, "Replay only: playback speed (1=real time, 0=as fast as possible)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Resolve the capture filename (if any) and re-parse flags that trail it.
	// This must happen before any flag value is read below — otherwise trailing
	// flags would be silently ignored (defaults applied) for everything below.
	var inName string
	if *portName == "" {
		if fs.NArg() < 1 {
			return nil, errNoTUISource
		}
		inName = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil { // allow flags after the filename
			return nil, err
		}
	}

	registry := ecm.NewRegistry()
	def, ok := registry.GetDefinition(*ecmPart)
	if !ok {
		return nil, fmt.Errorf("unknown ECM: %s", *ecmPart)
	}
	def = calibratedDef(def, *tps0, *tps100)

	return &tuiFlags{
		cfg:        decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert},
		registry:   registry,
		def:        def,
		ecmPart:    *ecmPart,
		promID:     *promID,
		minSamples: *minSamples,
		portName:   *portName,
		inName:     inName,
		speed:      *speed,
	}, nil
}

// cmdTUI launches the interactive dashboard — the default face of goaldl. It
// navigates between the sensor table, the BLM grid, the flag-data and
// error-code views, and a scrolling raw-byte history, driven by a
// stream.Session over either a live ECM (-p) or a replayed capture.
//
//	goaldl -p /dev/cu.usbserial-10
//	goaldl drive_4800.raw [-speed 2]
func cmdTUI(args []string) {
	cfg, err := resolveTUIFlags(args)
	if err != nil {
		if errors.Is(err, errNoTUISource) {
			fmt.Fprintln(os.Stderr, "No source. Give a port or a capture file:")
			fmt.Fprintln(os.Stderr, "  goaldl -p /dev/cu.usbserial-10")
			fmt.Fprintln(os.Stderr, "  goaldl drive_4800.raw")
			fmt.Fprintln(os.Stderr, "\nSee 'goaldl help' for the scripting commands (ports, record, decode, blm, …).")
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}

	var provider stream.Provider
	if cfg.portName != "" {
		provider = &stream.SerialProvider{Port: cfg.portName, Baud: cfg.cfg.BaudRate, Config: cfg.cfg}
	} else {
		data, err := os.ReadFile(cfg.inName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", cfg.inName, err)
			os.Exit(1)
		}
		provider = &stream.ReplayProvider{Data: data, Config: cfg.cfg, Speed: cfg.speed}
	}

	def := cfg.def
	session := stream.NewSession(provider, cfg.registry, cfg.ecmPart, cfg.promID)

	// Run the session in the background, delivering snapshots over a channel.
	// The emit blocks on the channel, so the session is paced by the UI.
	ctx, cancel := context.WithCancel(context.Background())
	// Small buffer to smooth brief UI stalls (slow SSH, flow control). The emit
	// still blocks once it fills, back-pressuring the provider — the buffer only
	// buys ~8 frames of slack, it doesn't guarantee the UART keeps draining.
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
		minSamples: cfg.minSamples,
		source:     session.Name(),
		snaps:      snaps,
		cancel:     cancel,
		grid:       blm.NewDefault(),
		intGrid:    blm.NewDefault(),
		o2Grid:     blm.NewDefault(),
		mins:       map[string]float64{},
		maxs:       map[string]float64{},
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
	viewINT
	viewO2
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
	grid          *blm.Grid          // BLM (long-term trim), closed-loop + BLM-enable gated
	intGrid       *blm.Grid          // INT (short-term trim), closed-loop gated
	o2Grid        *blm.Grid          // O2 voltage, ungated
	mins, maxs    map[string]float64 // per-sensor extrema since last reset
	hasExtrema    bool
	notice        string // transient footer message after a save/clear
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
			m.active = viewINT
		case "4":
			m.active = viewO2
		case "5":
			m.active = viewFlags
		case "6":
			m.active = viewCodes
		case "7":
			m.active = viewRaw
		case "tab", "right", "l":
			m.active = (m.active + 1) % viewCount
		case "shift+tab", "left", "h":
			m.active = (m.active + viewCount - 1) % viewCount
		case "s":
			m.notice = m.save()
		case "c":
			m.notice = m.clear()
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
		m.accumulate(s)
		return m, m.waitForSnapshot()

	case providerDoneMsg:
		m.done = true
	}
	return m, nil
}

// accumulate folds one snapshot into the consumer-side grids and per-sensor
// extrema. BLM gates on closed loop + block-learn enable; INT on closed loop;
// O2 is ungated. Value-based accumulation (INT/O2/extrema) requires a parseable
// frame, since it reads the decoded Sensors map.
func (m *tuiModel) accumulate(s stream.Snapshot) {
	ft := s.FuelTrim
	if ft.Recordable() {
		m.grid.Add(ft.RPM, ft.MapKPa, ft.BLM)
	}
	if !s.ParseOK {
		return
	}
	if ft.ClosedLoop {
		m.intGrid.Add(ft.RPM, ft.MapKPa, s.Sensors["integrator"])
	}
	m.o2Grid.Add(ft.RPM, ft.MapKPa, s.Sensors["oxygen_sensor"]/1000.0)
	for name, v := range s.Sensors {
		if cur, ok := m.mins[name]; !ok || v < cur {
			m.mins[name] = v
		}
		if cur, ok := m.maxs[name]; !ok || v > cur {
			m.maxs[name] = v
		}
	}
	m.hasExtrema = true
}

// save writes all three grids to timestamped files and returns a footer notice.
func (m *tuiModel) save() string {
	base, err := saveGrids(".", time.Now(), m.grid, m.intGrid, m.o2Grid, m.minSamples)
	if err != nil {
		return "save failed: " + err.Error()
	}
	return "saved BLM/INT/O2 → " + base + "_*.txt"
}

// clear resets state for the active tab: the viewed grid (BLM/INT/O2) or, on the
// sensor tab, the Min/Max extrema. Other tabs are a no-op (notice unchanged).
func (m *tuiModel) clear() string {
	switch m.active {
	case viewBLM:
		m.grid = blm.NewDefault()
		return "cleared BLM grid"
	case viewINT:
		m.intGrid = blm.NewDefault()
		return "cleared INT grid"
	case viewO2:
		m.o2Grid = blm.NewDefault()
		return "cleared O2 grid"
	case viewSensors:
		m.mins, m.maxs, m.hasExtrema = map[string]float64{}, map[string]float64{}, false
		return "reset min/max"
	}
	return m.notice
}

// saveGrids writes the BLM, INT, and O2 grids to three files in dir sharing one
// timestamp; returns the shared base name ("goaldl_<ts>").
func saveGrids(dir string, ts time.Time, blmG, intG, o2G *blm.Grid, minSamples int) (string, error) {
	base := "goaldl_" + ts.Format("20060102_150405")
	writeOne := func(suffix string, write func(io.Writer)) error {
		f, err := os.Create(filepath.Join(dir, base+"_"+suffix+".txt"))
		if err != nil {
			return err
		}
		write(f)
		return f.Close()
	}
	if err := writeOne("BLM", func(w io.Writer) { writeTrimGridFile(w, blmG, "BLM", minSamples) }); err != nil {
		return base, err
	}
	if err := writeOne("INT", func(w io.Writer) { writeTrimGridFile(w, intG, "INT", minSamples) }); err != nil {
		return base, err
	}
	if err := writeOne("O2", func(w io.Writer) { writeO2File(w, o2G) }); err != nil {
		return base, err
	}
	return base, nil
}

// writeTrimGridFile writes Samples + Wide Average + Correction for a 128-centered
// trim grid (BLM or INT), matching the `blm` command's file format.
func writeTrimGridFile(w io.Writer, g *blm.Grid, name string, minSamples int) {
	fmt.Fprint(w, g.RenderInt("Samples", g.Samples()))
	fmt.Fprintln(w)
	fmt.Fprint(w, g.RenderFloat("Wide Average "+name+" (target 128; >128 lean, <128 rich)", g.Average(), 1))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Correction factor = avg/128 (cells with <%d samples held at 1.000)\n", minSamples)
	fmt.Fprint(w, g.RenderFloat("", g.CorrectionAtLeast(minSamples), 3))
}

// writeO2File writes Samples + Wide Average (volts, 3 decimals). O2 is a
// voltage, not a trim multiplier, so there is no correction table.
func writeO2File(w io.Writer, g *blm.Grid) {
	fmt.Fprint(w, g.RenderInt("Samples", g.Samples()))
	fmt.Fprintln(w)
	fmt.Fprint(w, g.RenderFloat("Wide Average O2 (volts)", g.Average(), 3))
}

var (
	tabActive   = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	beatOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	beatBad     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	loopClosed  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green
	loopOpen    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // amber
)

func (m tuiModel) View() string {
	tabs := []string{"1 Sensors", "2 BLM", "3 INT", "4 O2", "5 Flags", "6 Codes", "7 Raw"}
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
		body = dimStyle.Render("\n  waiting for a parseable frame… (see 7 Raw for the byte stream)")
	case m.active == viewSensors:
		body = stream.SensorTableExtrema(m.lastGood.FrameEvent, m.def, m.mins, m.maxs)
	case m.active == viewBLM:
		body = stream.BLMBody(m.grid, m.lastGood.FrameEvent, m.minSamples)
	case m.active == viewINT:
		body = stream.INTBody(m.intGrid, m.lastGood.FrameEvent, m.minSamples, m.lastGood.Sensors["integrator"])
	case m.active == viewO2:
		body = stream.O2Body(m.o2Grid, m.lastGood.FrameEvent, m.lastGood.Sensors["oxygen_sensor"]/1000.0)
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
	footer := status
	if m.notice != "" {
		footer += "   " + m.notice
	}
	footer += "   " + dimStyle.Render("1-7/tab · s save · c clear · q quit")

	return header + "\n" + m.loopStatusLine() + "\n\n" + body + "\n\n" + footer
}

// loopStatusLine renders the persistent recording-state line shown on every
// tab, colouring the loop badge (green closed / amber open / dim unknown) from
// the latest parseable frame's fuel-trim state.
func (m tuiModel) loopStatusLine() string {
	ft := m.lastGood.FuelTrim
	badge := stream.LoopBadge(ft, m.hasGood)
	rest := strings.TrimPrefix(stream.LoopStatus(ft, m.hasGood), badge)
	var style lipgloss.Style
	switch {
	case !m.hasGood:
		style = dimStyle
	case ft.ClosedLoop:
		style = loopClosed
	default:
		style = loopOpen
	}
	return "  " + style.Render(badge) + rest
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
