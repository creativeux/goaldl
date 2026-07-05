package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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

	// Live sources always get a RecordSink so the `r` key can start and stop
	// raw capture mid-session; replay sources keep the provider pointer so the
	// space/+/- keys can pause and re-pace playback.
	var provider stream.Provider
	var replay *stream.ReplayProvider
	var recSink *stream.RecordSink
	if cfg.portName != "" {
		recSink = &stream.RecordSink{}
		provider = &stream.SerialProvider{Port: cfg.portName, Baud: cfg.cfg.BaudRate, Config: cfg.cfg, Sink: recSink}
	} else {
		data, err := os.ReadFile(cfg.inName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", cfg.inName, err)
			os.Exit(1)
		}
		replay = &stream.ReplayProvider{Data: data, Config: cfg.cfg, Speed: cfg.speed}
		provider = replay
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
	// errCh carries the session's terminal error (a failed port open/read) to
	// the model. Buffered(1) and always sent before snaps closes, so the reader
	// in waitForSnapshot never blocks and never misses it.
	errCh := make(chan error, 1)
	go func() {
		runErr := session.Run(ctx, func(s stream.Snapshot) {
			select {
			case snaps <- s:
			case <-ctx.Done():
			}
		})
		errCh <- runErr
		close(snaps)
	}()

	m := tuiModel{
		def:        def,
		minSamples: cfg.minSamples,
		source:     session.Name(),
		snaps:      snaps,
		errCh:      errCh,
		cancel:     cancel,
		replay:     replay,
		recSink:    recSink,
		grid:       blm.NewDefault(),
		intGrid:    blm.NewDefault(),
		o2Grid:     blm.NewDefault(),
		sparkGrid:  blm.NewSpark(),
		buf:        newFrameBuf(),
		mins:       map[string]float64{},
		maxs:       map[string]float64{},
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	cancel()
	if fm, ok := final.(tuiModel); ok {
		fm.closeOutputs()
		if fm.fatalErr != nil {
			// Reprint after the alt-screen tears down, so the diagnosis survives
			// the exit and is visible to a script that redirected stderr.
			fmt.Fprintf(os.Stderr, "goaldl: %v\n", fm.fatalErr)
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
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

// confirmKind is the pending destructive-action confirmation (a two-press modal).
type confirmKind int

const (
	confirmNone confirmKind = iota
	confirmQuit
	confirmClear
)

type view int

const (
	viewSensors view = iota
	viewBLM
	viewINT
	viewO2
	viewSpark
	viewFlags
	viewCodes
	viewRaw
	viewCount
)

// outputOp is which of the two output operations a picker will perform:
// Save Buffer (retroactive dump of the frame ring) or Log (forward streaming).
type outputOp int

const (
	opSaveBuffer outputOp = iota
	opLog
)

// fmtItem is one checkbox in an output picker: a stable id, a display label,
// whether it is currently selected, and whether it is disabled in the current
// context (shown dimmed with a note, not hidden — e.g. RAW on a replay source).
type fmtItem struct {
	id       string // "csv", "blm", "int", "o2", "spark", "raw"
	label    string
	on       bool
	disabled bool
	note     string // why it's disabled (shown dimmed beside the label)
}

// outputPicker is the modal opened by `s` (Save Buffer) and `r` (Log): a format
// checklist plus editable destination-directory and base-name fields, confirmed
// once. cursor indexes the items, then the dir field, then the name field. hint
// is a transient message (e.g. a name collision) shown until the next edit.
type outputPicker struct {
	op     outputOp
	items  []fmtItem
	cursor int
	dir    string
	name   string
	hint   string
}

// dirRow and nameRow are the cursor positions of the two editable path fields,
// just past the last checklist item (dir first, then name).
func (p *outputPicker) dirRow() int  { return len(p.items) }
func (p *outputPicker) nameRow() int { return len(p.items) + 1 }

// selected returns the ids of the checked, enabled items.
func (p *outputPicker) selected() []string {
	var out []string
	for _, it := range p.items {
		if it.on && !it.disabled {
			out = append(out, it.id)
		}
	}
	return out
}

// outputRecord is one written file, kept for the exit summary (Phase C.4) and
// the confirmation notice: the path and a human detail (rows / cells / bytes).
type outputRecord struct {
	name   string
	detail string
}

// gridSel pairs a grid's checklist id/suffix with its file writer, so Save
// Buffer can write any selected subset (delivering single-grid save, F18).
type gridSel struct {
	id     string
	suffix string
	write  func(io.Writer)
}

// allGridSels builds the writer for each of the four grids; the picker filters
// this by the selected ids.
func allGridSels(blmG, intG, o2G, sparkG *blm.Grid, minSamples int) []gridSel {
	return []gridSel{
		{"blm", "BLM", func(w io.Writer) { writeTrimGridFile(w, blmG, "BLM", minSamples) }},
		{"int", "INT", func(w io.Writer) { writeTrimGridFile(w, intG, "INT", minSamples) }},
		{"o2", "O2", func(w io.Writer) { writeO2File(w, o2G) }},
		{"spark", "SPARK", func(w io.Writer) { writeSparkFile(w, sparkG) }},
	}
}

// rawHistoryCap bounds the raw-frame ring; more than the widest terminal can
// show (the view itself caps at 14 columns, WinALDL-style).
const rawHistoryCap = 64

// snapshotMsg carries one processed frame into the update loop; providerDoneMsg
// signals the stream ended (replay finished or port closed) and carries the
// session's terminal error (nil on a clean end, context.Canceled on quit);
// noticeExpireMsg clears a self-expiring warning notice (the payload is the
// notice sequence number it was armed for, so a stale timer never clears a
// newer notice); tickMsg is the 1s heartbeat that drives staleness detection.
type (
	snapshotMsg     stream.Snapshot
	providerDoneMsg struct{ err error }
	noticeExpireMsg int
	tickMsg         time.Time
)

// noticeTTL is how long a no-op key warning (e.g. `r` during replay) stays in
// the footer before clearing itself.
const noticeTTL = 3 * time.Second

// staleAfter is how long a live stream may go without a new frame before the
// dashboard flags its data stale (~5 frames at the ~1.2s ALDL cadence).
const staleAfter = 6 * time.Second

// Free-running-knock detection window: judge over the last knockWindowSize
// parsed frames, but only once knockWindowMin have been seen; declare the
// counter free-running when at least knockFreeFrac of them carried a nonzero
// KNOCK_CNT delta. On the target vehicle the counter advances every frame
// (~100% nonzero); genuine ESC knock is nonzero on only a few frames a session,
// so the two regimes separate cleanly well away from the threshold.
const (
	knockWindowSize = 40
	knockWindowMin  = 20
	knockFreeFrac   = 0.5
)

type tuiModel struct {
	// config / wiring
	def        *ecm.Definition
	minSamples int
	source     string
	snaps      <-chan stream.Snapshot
	errCh      <-chan error // session's terminal error, read when snaps closes
	cancel     context.CancelFunc
	replay     *stream.ReplayProvider // pause/speed handle (nil when live)
	recSink    *stream.RecordSink     // raw-capture tee (nil when replay)

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
	sparkGrid     *blm.Grid          // knock-count deltas, ungated (WinALDL spark axes)
	knockPrev     float64            // last frame's cumulative KNOCK_CNT byte
	hasKnockBase  bool               // first parsed frame only sets the baseline
	mins, maxs    map[string]float64 // per-sensor extrema since last reset
	hasExtrema    bool

	// session safety (C.1–C.3): dirtyGrids is set when a grid accumulates and
	// cleared by a grid-inclusive Save Buffer; confirm is the pending destructive-
	// action modal (quit or clear), armed by the first keypress and carried out by
	// a second of the same key.
	dirtyGrids bool
	confirm    confirmKind

	notice     string         // transient footer message after a save/clear
	noticeSeq  int            // bumped on every notice change; guards expiry timers
	picker     *outputPicker  // modal output checklist (nil when inactive)
	buf        *frameBuf      // always-on decoded-frame ring for Save Buffer
	written    []outputRecord // files written this session (exit summary)
	recFile    *os.File       // open raw-capture target (nil when not logging raw)
	recName    string
	csvLog     *frameCSV // open CSV log (nil when not logging CSV)
	csvName    string
	logBase    string        // base path for the active Log's grid snapshot (at stop)
	logGridIDs []string      // grids selected for the active Log, written when it stops
	logStartAt time.Duration // frame-timeline position when the active Log started (for the REC clock)
	frameCount int
	done       bool
	fatalErr   error // session's terminal error (nil = clean end / user quit)

	// staleness (live only): lastFrameAt is when the newest snapshot arrived;
	// now is advanced by the 1s tick. A live stream that has gone quiet for
	// staleAfter is flagged stale (hollow heartbeat + footer age).
	lastFrameAt time.Time
	now         time.Time

	// free-running-knock detection: a sliding window over parsed frames records
	// whether each had a nonzero KNOCK_CNT delta. A high fraction means the
	// counter is free-running (a counter artifact, not knock) — see A.3.
	knockWindow      [knockWindowSize]bool
	knockWindowHead  int // next write position (ring)
	knockWindowCount int // frames seen, capped at knockWindowSize
	knockNonzero     int // count of true entries currently in the window

	// heartbeat health: a sliding window over recent frames tracks how many
	// failed to parse, so the heartbeat colour reflects *current* stream quality
	// rather than a cumulative ratio a rough start would drag down forever.
	healthWindow [healthWindowSize]bool // parseOK per recent frame
	healthHead   int
	healthCount  int // frames in the window, capped at healthWindowSize
	healthBad    int // count of !parseOK entries currently in the window

	// layout: showInfo expands the grid explainer accordion (`i`); scroll is the
	// vertical offset into a body taller than the terminal (`j`/`k`/↑/↓), so a
	// short terminal never scrolls the tab bar off the top.
	showInfo bool
	scroll   int
}

// waitForSnapshot blocks on the snapshot channel and delivers the next as a
// message. Re-issued after each snapshot to keep the stream flowing.
func (m tuiModel) waitForSnapshot() tea.Cmd {
	return func() tea.Msg {
		s, ok := <-m.snaps
		if !ok {
			return providerDoneMsg{err: <-m.errCh}
		}
		return snapshotMsg(s)
	}
}

func (m tuiModel) Init() tea.Cmd { return tea.Batch(m.waitForSnapshot(), m.tick()) }

// stale reports whether a live stream has gone quiet (no new frame for
// staleAfter) and for how long. It is false for a replay (paced playback and
// pause are expected quiet), before the first frame, and after the stream ends
// — those states have their own chrome. Pure over model fields, so the tests
// need no wall clock.
func (m tuiModel) stale() (bool, time.Duration) {
	if m.replay != nil || !m.hasFrame || m.done {
		return false, 0
	}
	age := m.now.Sub(m.lastFrameAt)
	return age >= staleAfter, age
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll = m.clampScroll(m.scroll) // the new height changes maxScroll
		// Clear on resize: we always emit a full terminal-height frame, but the
		// terminal may leave stale rows from the old size — wipe them so a shrunk
		// frame can't leave a frozen copy of the old footer behind.
		return m, tea.ClearScreen

	case tea.KeyMsg:
		// An open output picker captures every key (digits, q, … edit the name;
		// space toggles a box); only ctrl+c still quits. The mutating call is
		// sequenced into its own statement so m is copied for the return *after*
		// it runs (a call inside `return m, …` is unordered relative to reading m).
		if m.picker != nil {
			cmd := m.handlePickerKey(msg)
			return m, cmd
		}
		prevActive := m.active
		key := msg.String()
		// A pending confirm modal (quit/clear): ctrl+c always quits, the confirm's
		// own key carries out the action, and any other key cancels it — then falls
		// through so that key still does its normal job.
		if m.confirm != confirmNone {
			switch {
			case key == "ctrl+c":
				m.cancel()
				return m, tea.Quit
			case m.confirm == confirmQuit && key == "q":
				m.cancel()
				return m, tea.Quit
			case m.confirm == confirmClear && key == "c":
				m.confirm = confirmNone
				m.setNotice(m.clear())
				return m, nil
			default:
				m.confirm = confirmNone // cancel; the key is handled below
			}
		}
		switch key {
		case "ctrl+c":
			m.cancel() // the unconditional escape hatch
			return m, tea.Quit
		case "q":
			// Guard the quit when there is an open Log or unsaved grid data: the
			// first q opens the confirm modal, a second q quits. Clean state quits
			// at once.
			if m.logActive() || m.unsaved() {
				m.confirm = confirmQuit // View shows the confirm modal
				return m, nil
			}
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
			m.active = viewSpark
		case "6":
			m.active = viewFlags
		case "7":
			m.active = viewCodes
		case "8":
			m.active = viewRaw
		case "tab", "right":
			m.active = (m.active + 1) % viewCount
		case "shift+tab", "left":
			m.active = (m.active + viewCount - 1) % viewCount
		case "i":
			// Toggle the grid explainer accordion; height changes, so re-home the
			// scroll. A no-op on non-grid tabs (they render no explainer).
			m.showInfo = !m.showInfo
			m.scroll = 0
		case "down":
			m.scroll = m.clampScroll(m.scroll + 1)
		case "up":
			m.scroll = m.clampScroll(m.scroll - 1)
		case "s":
			m.openSaveBuffer()
		case "c":
			// Confirm before clearing (a modal, like quit); a no-op with a notice
			// on tabs / empty grids that have nothing to clear.
			if m.clearable() {
				m.confirm = confirmClear
				return m, nil
			}
			return m, m.warn("nothing to clear")
		case "l":
			cmd := m.toggleLog() // sequence the mutation before reading m
			return m, cmd
		case " ":
			if cmd := m.replayGuard(); cmd != nil {
				return m, cmd
			}
			m.replay.SetPaused(!m.replay.Paused())
		case "+", "=":
			cmd := m.adjustSpeed(2)
			return m, cmd
		case "-":
			cmd := m.adjustSpeed(0.5)
			return m, cmd
		}
		if m.active != prevActive {
			m.scroll = 0 // a fresh tab starts at the top
		}

	case snapshotMsg:
		s := stream.Snapshot(msg)
		m.latest = s
		m.hasFrame = true
		m.frameCount++
		m.lastFrameAt = time.Now()
		m.now = m.lastFrameAt
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
		m.pushHealth(s.ParseOK)
		m.accumulate(s)
		m.buf.push(m.bufFrame(s, frame))
		if m.csvLog != nil && s.ParseOK {
			// ParseOK rows only — parity with `monitor -csv`, which writes a
			// row only when the frame parses.
			m.csvLog.Write(s.Elapsed.Seconds(), s.Frame.ByteOffset, s.PROMOK, s.Sensors)
		}
		if m.recFile != nil {
			if err := m.recSink.Err(); err != nil {
				// The sink detached itself on a write error; the session keeps
				// running — close our handle and surface the stop.
				m.recFile.Close()
				m.recFile, m.recName = nil, ""
				m.setNotice("recording stopped: " + err.Error())
			}
		}
		if len(m.logGridIDs) > 0 {
			// Rewrite the aggregate tables every frame so the last complete
			// version survives a crash. Fail-soft: a write error detaches grid
			// logging and notices, but never kills the session.
			if err := m.rewriteLogGrids(); err != nil {
				m.logGridIDs, m.logBase = nil, ""
				m.setNotice("log grids stopped: " + err.Error())
			}
		}
		return m, m.waitForSnapshot()

	case providerDoneMsg:
		m.done = true
		// A clean end (nil) or the user's own quit (context.Canceled) is not an
		// error; anything else is a transport failure worth showing.
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			m.fatalErr = msg.err
		}

	case tickMsg:
		m.now = time.Time(msg)
		if m.done {
			return m, nil // stream ended — stop the idle staleness timer
		}
		return m, m.tick()

	case noticeExpireMsg:
		if int(msg) == m.noticeSeq {
			m.notice = ""
		}
	}
	return m, nil
}

// tick schedules the next 1s staleness heartbeat.
func (m tuiModel) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// setNotice replaces the footer notice, bumping the sequence number so any
// pending expiry timer armed for the previous notice becomes a no-op.
func (m *tuiModel) setNotice(text string) {
	m.notice = text
	m.noticeSeq++
}

// warn sets a footer notice that clears itself after noticeTTL — used for
// no-op key warnings (recording on replay, pause on live, …) so a stray
// keypress doesn't leave a stale message in the chrome.
func (m *tuiModel) warn(text string) tea.Cmd {
	m.setNotice(text)
	seq := m.noticeSeq
	return tea.Tick(noticeTTL, func(time.Time) tea.Msg { return noticeExpireMsg(seq) })
}

// accumulate folds one snapshot into the consumer-side grids and per-sensor
// extrema. BLM gates on closed loop + block-learn enable; INT on closed loop;
// O2 and spark are ungated. Value-based accumulation (INT/O2/spark/extrema)
// requires a parseable frame, since it reads the decoded Sensors map.
func (m *tuiModel) accumulate(s stream.Snapshot) {
	ft := s.FuelTrim
	if ft.Recordable() {
		m.grid.Add(ft.RPM, ft.MapKPa, ft.BLM)
		m.dirtyGrids = true
	}
	if !s.ParseOK {
		return
	}
	if ft.ClosedLoop {
		m.intGrid.Add(ft.RPM, ft.MapKPa, s.Sensors["integrator"])
	}
	m.o2Grid.Add(ft.RPM, ft.MapKPa, s.Sensors["oxygen_sensor"]/1000.0)
	m.dirtyGrids = true // O2 accumulates on every parseable frame
	// Spark bins per-frame deltas of the cumulative KNOCK_CNT byte (wraps at
	// 255). The first parsed frame only establishes the baseline — WinALDL
	// counts knocks during the session, not the counter's absolute value.
	knock := s.Sensors["knock_count"]
	if m.hasKnockBase {
		delta := math.Mod(knock-m.knockPrev+256, 256)
		m.pushKnock(delta > 0) // feed the free-running-counter detector
		if delta > 0 {
			m.sparkGrid.Add(ft.RPM, ft.MapKPa, delta)
		}
	}
	m.knockPrev, m.hasKnockBase = knock, true
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

// gridsHaveData reports whether any of the four grids currently holds samples.
func (m tuiModel) gridsHaveData() bool {
	return m.grid.TotalSamples() > 0 || m.intGrid.TotalSamples() > 0 ||
		m.o2Grid.TotalSamples() > 0 || m.sparkGrid.TotalSamples() > 0
}

// unsaved reports whether the grids hold data not yet written by a Save Buffer.
func (m tuiModel) unsaved() bool { return m.dirtyGrids && m.gridsHaveData() }

// quitGuardReason names what is at risk when a quit is held.
func (m tuiModel) quitGuardReason() string {
	switch {
	case m.logActive() && m.unsaved():
		return "A log is recording and grids hold unsaved data."
	case m.logActive():
		return "A log is still recording."
	default:
		return "Grids hold unsaved data."
	}
}

// confirmPanel is the centered destructive-action modal (quit or clear) shown
// while m.confirm is set — a bordered dialog over an otherwise-cleared screen, so
// the blocking decision isn't crowded into the footer. Keys are handled normally
// underneath (the confirm key carries out the action, anything else cancels).
func (m tuiModel) confirmPanel() string {
	var title, reason, keys string
	switch m.confirm {
	case confirmClear:
		title = "Clear " + m.clearTarget() + "?"
		reason = "This can't be undone."
		keys = "[c] clear  ·  any other key cancels"
	default: // confirmQuit
		title = "Quit?"
		reason = m.quitGuardReason()
		keys = "[q] quit  ·  [s] save  ·  any other key keeps working"
	}
	body := beatBad.Render(title) + "\n\n" + reason + "\n\n" + dimStyle.Render(keys)
	return m.modal(body)
}

// modal renders content as a bordered box centered over the frame area.
func (m tuiModel) modal(body string) string {
	box := modalStyle.Render(body)
	if w := m.contentWidth(); w > 0 && m.height > 0 {
		return lipgloss.Place(w, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// clearTarget names what [c] clears on the active tab (for the confirm modal).
func (m tuiModel) clearTarget() string {
	switch m.active {
	case viewBLM:
		return "BLM grid"
	case viewINT:
		return "INT grid"
	case viewO2:
		return "O2 grid"
	case viewSpark:
		return "SPARK grid"
	case viewSensors:
		return "sensor min/max"
	}
	return ""
}

// clearable reports whether the active tab has something to clear.
func (m tuiModel) clearable() bool {
	switch m.active {
	case viewBLM:
		return m.grid.TotalSamples() > 0
	case viewINT:
		return m.intGrid.TotalSamples() > 0
	case viewO2:
		return m.o2Grid.TotalSamples() > 0
	case viewSpark:
		return m.sparkGrid.TotalSamples() > 0
	case viewSensors:
		return m.hasExtrema
	}
	return false
}

// clear resets state for the active tab: the viewed grid (BLM/INT/O2/Spark) or,
// on the sensor tab, the Min/Max extrema. Clearing the spark grid keeps the knock
// baseline — a clear must not manufacture a phantom delta on the next frame.
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
	case viewSpark:
		m.sparkGrid = blm.NewSpark()
		return "cleared SPARK grid"
	case viewSensors:
		m.mins, m.maxs, m.hasExtrema = map[string]float64{}, map[string]float64{}, false
		return "reset min/max"
	}
	return m.notice
}

// defaultBase is the pre-filled base-name for the output picker: a timestamped
// goaldl_<ts> the operator can accept with one Enter.
func defaultBase() string { return "goaldl_" + time.Now().Format("20060102_150405") }

// currentDir is the pre-filled destination directory for the output picker —
// the working directory, editable in the picker so files can land elsewhere.
func currentDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// chopRune drops the last rune of s (backspace in the picker's text fields).
func chopRune(s string) string {
	if r := []rune(s); len(r) > 0 {
		return string(r[:len(r)-1])
	}
	return s
}

// contains reports whether id is in the selected-format list.
func contains(sel []string, id string) bool {
	for _, s := range sel {
		if s == id {
			return true
		}
	}
	return false
}

// bufFrame projects a snapshot into the compact record the ring retains. It
// reuses the caller's already-copied frame bytes and builds a fresh values slice
// in def.Parameters order, so the live Sensors map is not kept alive.
func (m tuiModel) bufFrame(s stream.Snapshot, frame []byte) bufFrame {
	bf := bufFrame{
		data:       frame,
		elapsedSec: s.Elapsed.Seconds(),
		byteOffset: s.Frame.ByteOffset,
		parseOK:    s.ParseOK,
		promOK:     s.PROMOK,
	}
	if s.ParseOK {
		vals := make([]float64, len(m.def.Parameters))
		for i, p := range m.def.Parameters {
			vals[i] = s.Sensors[p.Name]
		}
		bf.vals = vals
	}
	return bf
}

// gridItems is the four fuel-trim/spark grid checkboxes, shared by the Save
// Buffer and Log pickers (all default on).
func gridItems() []fmtItem {
	return []fmtItem{
		{id: "blm", label: "BLM grid", on: true},
		{id: "int", label: "INT grid", on: true},
		{id: "o2", label: "O2 grid", on: true},
		{id: "spark", label: "SPARK grid", on: true},
	}
}

// openSaveBuffer opens the Save Buffer picker: a decoded-only checklist (no RAW,
// which cannot be reconstructed from decoded frames) over the frame ring and the
// four grids. All boxes default on; the common case is Enter twice.
func (m *tuiModel) openSaveBuffer() {
	items := append([]fmtItem{{id: "csv", label: "Sensor CSV", on: true}}, gridItems()...)
	m.picker = &outputPicker{op: opSaveBuffer, items: items, dir: currentDir(), name: defaultBase()}
}

// logActive reports whether a Log session is running: a streaming output (raw
// or CSV) is open, or grids are pending a stop-time snapshot.
func (m tuiModel) logActive() bool {
	return m.recFile != nil || m.csvLog != nil || len(m.logGridIDs) > 0
}

// toggleLog stops an open Log, or opens the Log picker. A Log streams RAW/CSV
// forward and snapshots any selected grids when it stops. The Log picker offers
// the same outputs as Save Buffer plus RAW; RAW needs a live serial stream, so
// on a replay source it is shown disabled (not hidden) and defaults off.
func (m *tuiModel) toggleLog() tea.Cmd {
	if m.logActive() {
		m.stopLog()
		return nil
	}
	// Forward logging is live-only; on a replay the ring covers export via [s].
	if m.replay != nil {
		return m.warn("logging is live-only — use [s] save to export")
	}
	raw := fmtItem{id: "raw", label: "RAW bytes"}
	if m.recSink == nil {
		raw.disabled, raw.note = true, "live only"
	}
	items := append([]fmtItem{raw, {id: "csv", label: "Sensor CSV", on: true}}, gridItems()...)
	m.picker = &outputPicker{op: opLog, items: items, dir: currentDir(), name: defaultBase()}
	return nil
}

// stopLog closes any open Log streams, writes the snapshot of any grids the Log
// selected, records everything for the exit summary, and reports what was written.
func (m *tuiModel) stopLog() {
	var parts []string
	if m.recFile != nil {
		_, n := m.recSink.Set(nil)
		m.recFile.Close()
		detail := humanBytes(n)
		m.written = append(m.written, outputRecord{m.recName, detail})
		parts = append(parts, fmt.Sprintf("%s (%s)", m.recName, detail))
		m.recFile, m.recName = nil, ""
	}
	if m.csvLog != nil {
		rows := m.csvLog.Rows
		m.csvLog.Close()
		m.written = append(m.written, outputRecord{m.csvName, fmt.Sprintf("%d rows", rows)})
		parts = append(parts, fmt.Sprintf("%s (%d rows)", m.csvName, rows))
		m.csvLog, m.csvName = nil, ""
	}
	if len(m.logGridIDs) > 0 {
		base, ids := m.logBase, m.logGridIDs
		err := m.rewriteLogGrids() // final flush to capture the last frame
		m.logGridIDs, m.logBase = nil, ""
		if err != nil {
			parts = append(parts, "grids failed ("+err.Error()+")")
		} else {
			for _, g := range logGridSels(m, ids) {
				m.written = append(m.written, outputRecord{base + "_" + g.suffix + ".txt", "grid"})
			}
			parts = append(parts, fmt.Sprintf("%d grid(s)", len(ids)))
		}
	}
	if len(parts) > 0 {
		m.setNotice("stopped log: " + strings.Join(parts, ", "))
	}
}

// logGridSels returns the gridSel writers for the grid ids the active Log
// selected, reading the current grid pointers (so a mid-log clear is honoured).
func logGridSels(m *tuiModel, ids []string) []gridSel {
	var sels []gridSel
	for _, g := range allGridSels(m.grid, m.intGrid, m.o2Grid, m.sparkGrid, m.minSamples) {
		if contains(ids, g.id) {
			sels = append(sels, g)
		}
	}
	return sels
}

// rewriteLogGrids rewrites every grid file the active Log selected with the
// grids' current accumulated state — called on every frame so the last complete
// tables survive a crash. Each file is written atomically (temp + rename), so a
// crash mid-write leaves the previous complete version intact, never a torn one.
// A no-op when no grids are pending.
func (m *tuiModel) rewriteLogGrids() error {
	for _, g := range logGridSels(m, m.logGridIDs) {
		if err := atomicWriteFile(m.logBase+"_"+g.suffix+".txt", g.write); err != nil {
			return err
		}
	}
	return nil
}

// atomicWriteFile writes via a sibling temp file, fsyncs it, and renames it over
// path — so a reader (or a crash) never sees a partially-written file.
func atomicWriteFile(path string, write func(io.Writer)) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	write(f)
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// handlePickerKey routes keys while the output picker is open: ↑/↓ move the
// cursor across the checkboxes and the name field, space toggles the box under
// the cursor, printable runes edit the name (only on the name row), enter
// confirms, esc cancels, and only ctrl+c still quits the program.
func (m *tuiModel) handlePickerKey(msg tea.KeyMsg) tea.Cmd {
	p := m.picker
	switch msg.Type {
	case tea.KeyCtrlC:
		m.cancel()
		return tea.Quit
	case tea.KeyEscape:
		m.picker = nil
		m.setNotice("cancelled")
	case tea.KeyEnter:
		m.confirmPicker()
	case tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}
	case tea.KeyDown:
		if p.cursor < p.nameRow() {
			p.cursor++
		}
	case tea.KeySpace:
		if p.cursor < len(p.items) {
			if it := &p.items[p.cursor]; it.disabled {
				p.hint = it.label + " — " + it.note
			} else {
				it.on = !it.on
				p.hint = ""
			}
		}
	case tea.KeyBackspace:
		switch p.cursor {
		case p.dirRow():
			p.dir = chopRune(p.dir)
			p.hint = ""
		case p.nameRow():
			p.name = chopRune(p.name)
			p.hint = ""
		}
	case tea.KeyRunes:
		switch p.cursor {
		case p.dirRow():
			p.dir += string(msg.Runes)
			p.hint = ""
		case p.nameRow():
			p.name += string(msg.Runes)
			p.hint = ""
		}
	}
	return nil
}

// confirmPicker dispatches the picker's Enter: an empty name cancels, an empty
// selection keeps the picker open with a hint, otherwise the op-specific writer
// runs (a name collision also keeps the picker open — files are never
// overwritten).
func (m *tuiModel) confirmPicker() {
	p := m.picker
	name := strings.TrimSpace(p.name)
	if name == "" {
		m.picker = nil
		m.setNotice("cancelled")
		return
	}
	// Join the editable directory and name; an empty dir leaves the name verbatim
	// (working directory). A path typed into either field is still honoured.
	base := filepath.Join(strings.TrimSpace(p.dir), name)
	sel := p.selected()
	if len(sel) == 0 {
		p.hint = "nothing selected"
		return
	}
	switch p.op {
	case opSaveBuffer:
		m.confirmSaveBuffer(base, sel)
	case opLog:
		m.confirmLog(base, sel)
	}
}

// confirmSaveBuffer writes the selected grids and/or a Sensor CSV dumped from
// the frame ring, under the base name (dir "" so a bare name lands in the
// working directory and a typed path is honoured). Every target is pre-checked
// for existence before anything is written, so a collision aborts cleanly with
// the picker still open and no partial set on disk.
func (m *tuiModel) confirmSaveBuffer(base string, sel []string) {
	var grids []gridSel
	for _, g := range allGridSels(m.grid, m.intGrid, m.o2Grid, m.sparkGrid, m.minSamples) {
		if contains(sel, g.id) {
			grids = append(grids, g)
		}
	}
	csvName := base + ".csv"
	var targets []string
	for _, g := range grids {
		targets = append(targets, base+"_"+g.suffix+".txt")
	}
	if contains(sel, "csv") {
		targets = append(targets, csvName)
	}
	for _, t := range targets {
		if _, err := os.Stat(t); err == nil {
			m.picker.hint = "exists — edit the name"
			return
		}
	}
	if len(grids) > 0 {
		if err := saveGrids("", base, grids); err != nil {
			if errors.Is(err, fs.ErrExist) {
				m.picker.hint = "exists — edit the name"
				return
			}
			m.picker = nil
			m.setNotice("save failed: " + err.Error())
			return
		}
		for _, g := range grids {
			m.written = append(m.written, outputRecord{base + "_" + g.suffix + ".txt", ""})
		}
		m.dirtyGrids = false // grids are now on disk
	}
	if contains(sel, "csv") {
		c, err := newFrameCSV(csvName, m.def)
		if err != nil {
			m.picker = nil
			m.setNotice("csv failed: " + err.Error())
			return
		}
		for _, f := range m.buf.frames() {
			c.WriteRow(f)
		}
		rows := c.Rows
		c.Close()
		m.written = append(m.written, outputRecord{csvName, fmt.Sprintf("%d rows", rows)})
	}
	m.picker = nil
	m.setNotice(fmt.Sprintf("saved %d file(s) (%s)", len(targets), base))
}

// confirmLog opens forward-streaming writers (RAW/CSV) for the selected formats
// under the base name, and remembers any selected grids to snapshot when the Log
// stops (grids are session aggregates, not a stream). Every target — including
// the deferred grid files — is pre-checked so a collision keeps the picker open.
func (m *tuiModel) confirmLog(base string, sel []string) {
	rawName, csvName := base+".raw", base+".csv"
	var gridIDs []string
	var targets []string
	if contains(sel, "raw") {
		targets = append(targets, rawName)
	}
	if contains(sel, "csv") {
		targets = append(targets, csvName)
	}
	for _, g := range allGridSels(m.grid, m.intGrid, m.o2Grid, m.sparkGrid, m.minSamples) {
		if contains(sel, g.id) {
			gridIDs = append(gridIDs, g.id)
			targets = append(targets, base+"_"+g.suffix+".txt")
		}
	}
	for _, t := range targets {
		if _, err := os.Stat(t); err == nil {
			m.picker.hint = "exists — edit the name"
			return
		}
	}
	var parts []string
	if contains(sel, "raw") {
		f, err := os.OpenFile(rawName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			m.picker = nil
			m.setNotice("log raw failed: " + err.Error())
			return
		}
		m.recSink.Set(f)
		m.recFile, m.recName = f, rawName
		parts = append(parts, rawName)
	}
	if contains(sel, "csv") {
		c, err := newFrameCSV(csvName, m.def)
		if err != nil {
			m.picker = nil
			m.setNotice("log csv failed: " + err.Error())
			return
		}
		m.csvLog, m.csvName = c, csvName
		parts = append(parts, csvName)
	}
	m.logBase, m.logGridIDs = base, gridIDs
	m.logStartAt = m.latest.Elapsed // anchor the REC clock
	if len(gridIDs) > 0 {
		// Write the tables once now (and on every frame after) so the latest
		// complete version is always on disk — crash-proof.
		if err := m.rewriteLogGrids(); err != nil {
			m.logGridIDs, m.logBase = nil, ""
			m.picker = nil
			m.setNotice("log grids failed: " + err.Error())
			return
		}
		parts = append(parts, fmt.Sprintf("%d grid(s) (live)", len(gridIDs)))
	}
	m.picker = nil
	m.setNotice("logging → " + strings.Join(parts, ", "))
}

// replayGuard returns a non-nil self-expiring warning command when the
// pause/speed keys cannot act (live source, or unpaced -speed 0); nil means
// the keys may proceed.
func (m *tuiModel) replayGuard() tea.Cmd {
	if m.replay == nil {
		return m.warn("pause/speed are replay-only")
	}
	if m.replay.CurrentSpeed() == 0 {
		return m.warn("unpaced replay (-speed 0)")
	}
	return nil
}

// adjustSpeed scales the replay rate by factor, clamped to 0.25×–16×.
func (m *tuiModel) adjustSpeed(factor float64) tea.Cmd {
	if cmd := m.replayGuard(); cmd != nil {
		return cmd
	}
	v := m.replay.CurrentSpeed() * factor
	v = math.Max(0.25, math.Min(16, v))
	m.replay.SetSpeed(v)
	// No notice — the legend's [±] speed (N×) reflects the change live.
	return nil
}

// closeOutputs closes any recording or CSV file still open when the program
// exits. Called after the session context is cancelled, with the sink
// detached first so the provider goroutine cannot write to a closed file.
func (m tuiModel) closeOutputs() {
	if m.recFile != nil {
		m.recSink.Set(nil)
		m.recFile.Close()
	}
	if m.csvLog != nil {
		m.csvLog.Close()
	}
	// Final flush of the active Log's grids so a clean quit captures the last
	// frame (errors are unreportable post-teardown; the previous per-frame write
	// is already durable on disk).
	m.rewriteLogGrids()
}

// humanBytes formats a byte count for the footer's recording segment.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f kB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// saveGrids writes the selected grids to files in dir sharing the caller-chosen
// base name (`<base>_BLM.txt`, …). Files are created exclusively; every target
// is checked up front so a name collision aborts cleanly (overwriting nothing),
// and a mid-write failure unlinks the files already created this call — either
// way no partial set is left behind. The caller passes only the grids it wants
// (see allGridSels), delivering single-grid save.
func saveGrids(dir, base string, files []gridSel) error {
	for _, fl := range files {
		if _, err := os.Stat(filepath.Join(dir, base+"_"+fl.suffix+".txt")); err == nil {
			return fmt.Errorf("%s_%s.txt: %w", base, fl.suffix, fs.ErrExist)
		}
	}
	var written []string
	for _, fl := range files {
		path := filepath.Join(dir, base+"_"+fl.suffix+".txt")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			written = append(written, path)
			fl.write(f)
			err = f.Close()
		}
		if err != nil {
			// A mid-write failure (disk full, media pulled) must not leave a
			// half-written set behind; unlink what this call already created.
			for _, p := range written {
				os.Remove(p)
			}
			return err
		}
	}
	return nil
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

// writeSparkFile writes Samples (frames with knock) + Knock counts (the grid's
// Sum of per-frame KNOCK_CNT deltas). Counts, not a trim — no correction table.
func writeSparkFile(w io.Writer, g *blm.Grid) {
	fmt.Fprint(w, g.RenderInt("Samples (frames with knock)", g.Samples()))
	fmt.Fprintln(w)
	fmt.Fprint(w, g.RenderFloat("Knock counts (delta of KNOCK_CNT)", g.Sum(), 0))
}

var (
	tabActive   = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	offStyle    = lipgloss.NewStyle().Faint(true).Strikethrough(true)            // a disabled key hint
	beatOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	beatWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))            // amber
	beatBad     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	loopClosed  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green
	loopOpen    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // amber
	brandStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // GoALDL logo (cyan; not reverse, so it doesn't read as a tab)
	modalStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(1, 3)
)

func (m tuiModel) View() string {
	if m.fatalErr != nil {
		return m.padHeight(m.fitWidth(m.errorPanel()))
	}
	if m.confirm != confirmNone {
		return m.padHeight(m.fitWidth(m.confirmPanel()))
	}
	if m.picker != nil {
		return m.padHeight(m.fitWidth(m.modal(m.pickerView())))
	}
	// Grid tabs carry a per-grid accumulation dot (● accumulating / ○ frozen by
	// loop gating), so which grids are learning reads straight off the tab bar —
	// no separate rec-dots row. BLM needs closed-loop + block-learn enable, INT
	// needs closed-loop, O2/Spark are always live once a good frame arrives.
	// Title bar: the GoALDL brand + a compact session status — the signal dot
	// (connection + parse-quality colour), the buffer fill, and (only while a Log
	// is running) the REC clock, then any transient notice. Loop state and the
	// elapsed clock are intentionally omitted (loop state still reads off the tab
	// dots). The mode badge isn't here: live is unbadged; replay's leads its row.
	status := dimStyle.Render("Signal:") + " " + m.signalDot()
	if m.done {
		status += "   " + dimStyle.Render("(stream ended)")
	}
	if stale, age := m.stale(); stale {
		status += "   " + beatBad.Render(fmt.Sprintf("no data %.0fs", age.Seconds()))
	}
	status += m.sessionChrome()
	if m.notice != "" {
		status += "   " + m.notice
	}
	// Title bar: GoALDL flush left, the status block flush right.
	brand := brandStyle.Render("GoALDL")
	gap := 3
	if w := m.contentWidth(); w > 0 {
		if g := w - ansi.StringWidth(brand) - ansi.StringWidth(status); g > gap {
			gap = g
		}
	}
	titleBar := brand + strings.Repeat(" ", gap) + status

	// Grid tabs carry a per-grid accumulation dot (● accumulating / ○ frozen by
	// loop gating), so which grids are learning reads straight off the tab bar.
	// BLM needs closed-loop + block-learn enable, INT needs closed-loop, O2/Spark
	// are always live once a good frame arrives.
	tabs := []string{"1 Sensors", "2 BLM", "3 INT", "4 O2", "5 Spark", "6 Flags", "7 Codes", "8 Raw"}
	rendered := make([]string, len(tabs))
	for i, t := range tabs {
		t += m.tabDot(view(i))
		if view(i) == m.active {
			rendered[i] = tabActive.Render(t)
		} else {
			rendered[i] = tabInactive.Render(t)
		}
	}
	// Blank line between the title bar and the tabs, so the brand reads as a
	// header rather than a tab.
	header := titleBar + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, rendered...)

	keys := m.keyLegend() // already per-segment styled
	// Bottom bar: (replay only) a playback-nav row, then the live legend.
	var footerRows []string
	if m.replay != nil {
		footerRows = append(footerRows, m.replayNav())
	}
	footerRows = append(footerRows, keys)
	footer := strings.Join(footerRows, "\n")

	// Title bar + tabs pinned top; one blank line lets the body breathe; the body;
	// a blank line; the footer pinned bottom. clampBody pads the body so the whole
	// frame is exactly the terminal height — the footer sits on the last rows every
	// render, so a resize can't leave a frozen copy behind.
	frame := header + "\n\n" + m.clampBody(m.activeBody()) + "\n\n" + footer
	return m.padHeight(m.fitWidth(frame))
}

// padHeight makes the frame exactly the terminal height — padding short frames
// (e.g. the error panel) with blank lines and clamping over-tall ones — so every
// screen row is written each render and a resize can't leave stale rows behind.
// No-op before the first WindowSizeMsg (height 0).
func (m tuiModel) padHeight(s string) string {
	if m.height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:m.height], "\n")
}

// fitWidth truncates every line of the frame to the terminal width, appending a
// › cue when a line is cut, so nothing soft-wraps (a wrapped chrome line would
// push the tab bar off the top, defeating the height clamp). ANSI-aware: styling
// escapes pass through and are reset at the cut. The grid/sensor bodies already
// truncate at column boundaries; this is the catch-all for the styled chrome and
// prose bodies. No-op before the first WindowSizeMsg (width 0).
func (m tuiModel) fitWidth(s string) string {
	w := m.contentWidth()
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = ansi.Truncate(ln, w, "›")
	}
	return strings.Join(lines, "\n")
}

// activeBody renders the current tab's content (no chrome). The grid tabs honour
// the info accordion (m.showInfo); the Spark tab also carries the free-running
// warning verdict.
func (m tuiModel) activeBody() string {
	switch {
	case !m.hasFrame:
		return dimStyle.Render("\n  waiting for frames…")
	case m.active == viewRaw:
		return m.rawView()
	case !m.hasGood:
		return dimStyle.Render("\n  waiting for a parseable frame… (see 8 Raw for the byte stream)")
	case m.active == viewSensors:
		return stream.SensorTableExtrema(m.lastGood.FrameEvent, m.def, m.mins, m.maxs, m.contentWidth())
	case m.active == viewBLM:
		return stream.BLMBodyDash(m.grid, m.lastGood.FrameEvent, m.minSamples, m.contentWidth(), m.showInfo)
	case m.active == viewINT:
		return stream.INTBody(m.intGrid, m.lastGood.FrameEvent, m.minSamples, m.showInfo, m.contentWidth())
	case m.active == viewO2:
		return stream.O2Body(m.o2Grid, m.lastGood.FrameEvent, m.showInfo, m.contentWidth())
	case m.active == viewSpark:
		return stream.SparkBody(m.sparkGrid, m.lastGood.FrameEvent, m.knockFreeRunning(), m.showInfo, m.contentWidth())
	case m.active == viewFlags:
		return stream.FlagsBody(m.lastGood.Flags)
	case m.active == viewCodes:
		return stream.CodesBody(m.lastGood.Codes)
	}
	return ""
}

// keyLegend is the always-present live key hint — the primary chrome, shown in
// both modes. Live-only keys that don't apply in replay ([l] log) render struck
// through (disabled; invoking them warns); [c] is labelled by what it clears and
// hidden where it's a no-op; [i] info only appears on grid tabs. The replay
// playback keys are a separate row added below only in replay (see replayNav).
func (m tuiModel) keyLegend() string {
	type seg struct {
		text string
		on   bool
	}
	segs := []seg{{"[s] save", true}}
	if c := m.clearLabel(); c != "" {
		segs = append(segs, seg{c, true})
	}
	if isGridTab(m.active) {
		segs = append(segs, seg{"[i] info", true})
	}
	logText := "[l] log"
	if m.logActive() {
		logText = "[l] stop log"
	}
	segs = append(segs, seg{logText, m.replay == nil}) // live-only: disabled on replay
	segs = append(segs, seg{"[q] quit", true})

	parts := make([]string, len(segs))
	for i, s := range segs {
		if s.on {
			parts[i] = dimStyle.Render(s.text)
		} else {
			parts[i] = offStyle.Render(s.text)
		}
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

// replayNav is the extra playback-control row shown only in replay mode, below
// the live legend — replay is a rare advanced/debug mode, so its keys are added
// rather than displacing the live chrome.
func (m tuiModel) replayNav() string {
	return dimStyle.Render("(Replay)  [space] pause · " + m.speedLabel())
}

// formatElapsed renders a stream position as m:ss (or h:mm:ss past an hour).
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// clearLabel names what [c] clears on the active tab (the grid, or the sensor
// min/max), or "" where clear is a no-op (flags/codes/raw) so [c] is hidden.
func (m tuiModel) clearLabel() string {
	switch m.active {
	case viewBLM:
		return "[c] clear BLM"
	case viewINT:
		return "[c] clear INT"
	case viewO2:
		return "[c] clear O2"
	case viewSpark:
		return "[c] clear SPARK"
	case viewSensors:
		return "[c] reset min/max"
	}
	return ""
}

// speedLabel is the replay playback-speed hint carrying the current speed; 0
// (an unpaced -speed 0 replay) reads as (max).
func (m tuiModel) speedLabel() string {
	if sp := m.replay.CurrentSpeed(); sp == 0 {
		return "[±] speed (max)"
	}
	return fmt.Sprintf("[±] speed (%g×)", m.replay.CurrentSpeed())
}

// maxContentWidth caps how wide the frame renders, so on a wide terminal the
// content (and the right-justified status) stays in a readable column near the
// left rather than stretching edge to edge. Sized to the widest body — the spark
// grid (WinALDL's 15 MAP columns), which renders ~84 cols but needs 86 for its
// column-fit logic to show every column without its › truncation cue. Everything
// else (tabs, tables, legends) is narrower, so nothing truncates.
const maxContentWidth = 86

// contentWidth is the effective render width — the terminal width, capped at
// maxContentWidth. 0 before the first WindowSizeMsg (unbounded, as the builders
// expect), so the initial frame renders in full.
func (m tuiModel) contentWidth() int {
	switch {
	case m.width <= 0:
		return 0
	case m.width < maxContentWidth:
		return m.width
	default:
		return maxContentWidth
	}
}

// chromeHeight is the non-body height of a frame: the title bar, a blank, the
// tab bar, a blank above the body, a blank below it, and the bottom bar (key
// legend = 1 row live; replay adds a playback-nav row = 2).
func (m tuiModel) chromeHeight() int {
	if m.replay != nil {
		return 7
	}
	return 6
}

// bodyBudget is how many lines the body may occupy after the fixed chrome. It is
// unbounded before the first WindowSizeMsg (m.height 0), so the initial frame
// renders in full.
func (m tuiModel) bodyBudget() int {
	if m.height <= 0 {
		return 1 << 30
	}
	if b := m.height - m.chromeHeight(); b > 1 {
		return b
	}
	return 1
}

// maxScroll is the largest useful scroll offset for the active body at the
// current size (0 when it fits). One line of the budget is reserved for the
// scroll status when the body overflows.
func (m tuiModel) maxScroll() int {
	budget := m.bodyBudget()
	lines := strings.Count(m.activeBody(), "\n") + 1
	if lines <= budget {
		return 0
	}
	win := budget - 1
	if win < 1 {
		win = 1
	}
	return lines - win
}

// clampScroll keeps a proposed scroll offset within [0, maxScroll].
func (m tuiModel) clampScroll(s int) int {
	if s < 0 {
		return 0
	}
	if max := m.maxScroll(); s > max {
		return max
	}
	return s
}

// clampBody fits body to exactly bodyBudget lines. When it overflows, it shows a
// scroll window (offset m.scroll) plus a one-line position/hint status; when it
// fits, it pads with blank lines. Either way the body region is exactly the
// budget, so the whole frame is exactly the terminal height and the footer sits
// on the last row every render (no floating/ghosting on resize). No padding
// before the first WindowSizeMsg (budget is unbounded).
func (m tuiModel) clampBody(body string) string {
	budget := m.bodyBudget()
	lines := strings.Split(body, "\n")
	if m.height <= 0 {
		return body
	}
	if len(lines) <= budget {
		for len(lines) < budget {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	}
	win := budget - 1
	if win < 1 {
		win = 1
	}
	maxScroll := len(lines) - win // local; avoids recomputing the body
	s := m.scroll
	if s > maxScroll {
		s = maxScroll
	}
	if s < 0 {
		s = 0
	}
	shown := strings.Join(lines[s:s+win], "\n")
	status := dimStyle.Render(fmt.Sprintf("  [↑/↓] scroll · lines %d–%d of %d", s+1, s+win, len(lines)))
	return shown + "\n" + status
}

// isGridTab reports whether v is one of the RPM×MAP grid tabs (the ones with an
// info-accordion explainer).
func isGridTab(v view) bool {
	return v == viewBLM || v == viewINT || v == viewO2 || v == viewSpark
}

// sessionChrome renders the buffer/logging/playback segments of the status: the
// frame-buffer fill %, a red ● REC clock while a Log is running (the recording
// time — data-timeline duration since it started), and the replay pause state.
func (m tuiModel) sessionChrome() string {
	out := "   " + dimStyle.Render(fmt.Sprintf("buf %d%%", m.buf.fillPct()))
	if m.logActive() {
		out += "   " + beatBad.Render("● REC "+formatElapsed(m.latest.Elapsed-m.logStartAt))
	}
	// Paused is a prominent state, so it stays in the status row; the running
	// speed is a control setting and rides next to its key in the legend instead.
	if m.replay != nil && m.replay.Paused() {
		out += "   " + loopOpen.Render("⏸ PAUSED")
	}
	return out
}

// pickerView renders the open output picker as the body: a format checklist
// with a cursor marker, the editable name field, the resolved destination
// directory (F17), and any transient hint (e.g. a name collision).
func (m tuiModel) pickerView() string {
	p := m.picker
	title := "Save Buffer — pick outputs"
	if p.op == opLog {
		title = "Log — pick outputs (streams to disk)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", brandStyle.Render(title))
	for i, it := range p.items {
		box := "[ ]"
		if it.on {
			box = "[x]"
		}
		cur := "  "
		if p.cursor == i {
			cur = "▸ "
		}
		if it.disabled {
			// Dimmed with the reason — visible but not selectable.
			fmt.Fprintf(&b, "%s\n", dimStyle.Render(fmt.Sprintf("%s%s %s (%s)", cur, box, it.label, it.note)))
			continue
		}
		fmt.Fprintf(&b, "%s%s %s\n", cur, box, it.label)
	}
	// Two editable path fields; the caret ▌ marks whichever the cursor is on.
	field := func(row int, label, val string) string {
		cur, caret := "  ", ""
		if p.cursor == row {
			cur, caret = "▸ ", "▌"
		}
		return fmt.Sprintf("%s%s %s%s\n", cur, label, val, caret)
	}
	b.WriteString("\n" + field(p.dirRow(), "dir: ", p.dir))
	b.WriteString(field(p.nameRow(), "name:", p.name))
	if p.hint != "" {
		fmt.Fprintf(&b, "\n%s\n", beatBad.Render(p.hint))
	}
	b.WriteString("\n" + dimStyle.Render("[↑↓] move · [space] toggle · [enter] confirm · [esc] cancel"))
	return b.String()
}

// styledLoopBadge is the loop-state word (CLOSED LOOP / OPEN LOOP / LOOP —) with
// a leading state circle — filled ● for closed loop, hollow ○ for open loop —
// coloured from the latest parseable frame (green closed / amber open / dim
// unknown). It leads the footer status line.
func (m tuiModel) styledLoopBadge() string {
	ft := m.lastGood.FuelTrim
	badge := stream.LoopBadge(ft, m.hasGood)
	switch {
	case !m.hasGood:
		return dimStyle.Render(badge)
	case ft.ClosedLoop:
		return loopClosed.Render("● " + badge)
	default:
		return loopOpen.Render("○ " + badge)
	}
}

// tabDot is the per-grid accumulation indicator appended to a grid tab's label:
// ● when that grid is currently learning, ○ when loop gating has it frozen, and
// "" for non-grid tabs. BLM needs closed loop + block-learn enable, INT needs
// closed loop, O2/Spark accumulate whenever a good frame is present.
func (m tuiModel) tabDot(v view) string {
	ft := m.lastGood.FuelTrim
	on := false
	switch v {
	case viewBLM:
		on = m.hasGood && ft.ClosedLoop && ft.BLMEnabled
	case viewINT:
		on = m.hasGood && ft.ClosedLoop
	case viewO2, viewSpark:
		on = m.hasGood
	default:
		return ""
	}
	if on {
		return " ●"
	}
	return " ○"
}

// healthLevel classifies recent stream quality from the sliding parse window —
// pure over model fields, so the colour choice is unit-testable (lipgloss strips
// colour off a TTY, so asserting the rendered glyph can't distinguish styles).
type healthLevel int

const (
	healthNone healthLevel = iota // no frames yet
	healthGood                    // essentially all recent frames parse → green
	healthWarn                    // some recent loss → amber
	healthBadL                    // heavy recent loss → red
)

// Heartbeat window and colour thresholds on the recent good-frame fraction.
const (
	healthWindowSize = 30   // recent frames the heartbeat colour reflects
	beatGreenFrac    = 0.95 // ≥ this recent good fraction → green (≤1 bad in 30)
	beatWarnFrac     = 0.80 // ≥ this → amber; below → red
)

func (m tuiModel) healthLevel() healthLevel {
	if m.healthCount == 0 {
		return healthNone
	}
	switch goodFrac := float64(m.healthCount-m.healthBad) / float64(m.healthCount); {
	case goodFrac >= beatGreenFrac:
		return healthGood
	case goodFrac >= beatWarnFrac:
		return healthWarn
	default:
		return healthBadL
	}
}

// signalDot is the signal-quality indicator: a filled ● whose colour is the
// recent parse quality (green/amber/red). It reflects live reception on a live
// source and — since replay re-processes the recorded frames through the same
// health window — replays the signal quality that was actually captured. A
// hollow ○ means no signal: dim before the first frame, red once a *live* stream
// that was flowing goes stale (replay is stale-exempt, so it never shows this).
func (m tuiModel) signalDot() string {
	if stale, _ := m.stale(); stale {
		return beatBad.Render("○") // a live stream that went quiet
	}
	switch m.healthLevel() {
	case healthGood:
		return beatOK.Render("●")
	case healthWarn:
		return beatWarn.Render("●")
	case healthBadL:
		return beatBad.Render("●")
	default:
		return dimStyle.Render("○") // no frames yet
	}
}

// pushHealth slides the heartbeat health window by one frame, keeping healthBad
// in sync as old entries are evicted.
func (m *tuiModel) pushHealth(ok bool) {
	if m.healthCount == healthWindowSize && !m.healthWindow[m.healthHead] {
		m.healthBad-- // evicting a bad entry
	}
	m.healthWindow[m.healthHead] = ok
	if !ok {
		m.healthBad++
	}
	m.healthHead = (m.healthHead + 1) % healthWindowSize
	if m.healthCount < healthWindowSize {
		m.healthCount++
	}
}

// errorPanel renders the full-screen diagnosis shown when the session dies with
// a transport error (typically a serial open/read failure). It replaces the tab
// view — there is no data to show. Serial hints are offered only for a live
// source.
func (m tuiModel) errorPanel() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  %s\n\n", beatBad.Render("⚠ Cannot read from "+m.source))
	fmt.Fprintf(&b, "  %v\n\n", m.fatalErr)
	if m.replay == nil { // live source: the failure is almost always the port/cable
		fmt.Fprintf(&b, "  %s\n", dimStyle.Render("• Check the cable and run:  goaldl ports"))
		fmt.Fprintf(&b, "  %s\n", dimStyle.Render("• Wrong baud rate?  add  -b 2400"))
		fmt.Fprintf(&b, "  %s\n\n", dimStyle.Render("• Non-inverting cable?  add  -invert"))
	}
	fmt.Fprintf(&b, "  %s", dimStyle.Render("[q] quit"))
	return b.String()
}

// pushKnock slides the free-running-knock window by one parsed frame, recording
// whether it carried a nonzero KNOCK_CNT delta and keeping knockNonzero in sync.
func (m *tuiModel) pushKnock(nonzero bool) {
	if m.knockWindowCount == knockWindowSize && m.knockWindow[m.knockWindowHead] {
		m.knockNonzero-- // evicting a nonzero entry
	}
	m.knockWindow[m.knockWindowHead] = nonzero
	if nonzero {
		m.knockNonzero++
	}
	m.knockWindowHead = (m.knockWindowHead + 1) % knockWindowSize
	if m.knockWindowCount < knockWindowSize {
		m.knockWindowCount++
	}
}

// knockFreeRunning reports whether KNOCK_CNT is advancing on most recent frames
// — the signature of a free-running counter (not a knock signal) on this
// vehicle. False until knockWindowMin frames are seen, so it never warns on the
// first second of data.
func (m tuiModel) knockFreeRunning() bool {
	if m.knockWindowCount < knockWindowMin {
		return false
	}
	return float64(m.knockNonzero)/float64(m.knockWindowCount) >= knockFreeFrac
}

func (m tuiModel) rawView() string {
	head := fmt.Sprintf("  offset %d   %s\n\n", m.latest.Frame.ByteOffset, promMark(m.latest.PROMOK))
	return head + stream.RawHistory(m.def.ByteLabels, m.history, m.contentWidth())
}

func promMark(ok bool) string {
	if ok {
		return "PROM ✓"
	}
	return "PROM ✗"
}
