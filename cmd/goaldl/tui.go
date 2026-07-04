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
		replay:     replay,
		recSink:    recSink,
		grid:       blm.NewDefault(),
		intGrid:    blm.NewDefault(),
		o2Grid:     blm.NewDefault(),
		sparkGrid:  blm.NewSpark(),
		mins:       map[string]float64{},
		maxs:       map[string]float64{},
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	cancel()
	if fm, ok := final.(tuiModel); ok {
		fm.closeOutputs()
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

// promptTarget names which file-producing action an open filename prompt will
// perform on confirm.
type promptTarget int

const (
	promptSave promptTarget = iota
	promptRecord
	promptCSV
)

// promptState is the modal filename editor opened by `s`, `r`, and `d`: buf is
// the editable base name (extensions/suffixes are appended by the action);
// hint is a transient message (e.g. name collision) shown until the next edit.
type promptState struct {
	target promptTarget
	buf    string
	hint   string
}

// rawHistoryCap bounds the raw-frame ring; more than the widest terminal can
// show (the view itself caps at 14 columns, WinALDL-style).
const rawHistoryCap = 64

// snapshotMsg carries one processed frame into the update loop; providerDoneMsg
// signals the stream ended (replay finished or port closed); noticeExpireMsg
// clears a self-expiring warning notice (the payload is the notice sequence
// number it was armed for, so a stale timer never clears a newer notice).
type (
	snapshotMsg     stream.Snapshot
	providerDoneMsg struct{}
	noticeExpireMsg int
)

// noticeTTL is how long a no-op key warning (e.g. `r` during replay) stays in
// the footer before clearing itself.
const noticeTTL = 3 * time.Second

type tuiModel struct {
	// config / wiring
	def        *ecm.Definition
	minSamples int
	source     string
	snaps      <-chan stream.Snapshot
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
	notice        string       // transient footer message after a save/clear
	noticeSeq     int          // bumped on every notice change; guards expiry timers
	prompt        *promptState // modal filename editor (nil when inactive)
	recFile       *os.File     // open raw-capture target (nil when not recording)
	recName       string
	csvLog        *frameCSV // open CSV log (nil when not logging)
	csvName       string
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
		// An open filename prompt captures every key (digits, q, … type into
		// the buffer); only ctrl+c still quits. The mutating call is sequenced
		// into its own statement so m is copied for the return *after* it runs
		// (a call inside `return m, …` is unordered relative to reading m).
		if m.prompt != nil {
			cmd := m.handlePromptKey(msg)
			return m, cmd
		}
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
			m.active = viewSpark
		case "6":
			m.active = viewFlags
		case "7":
			m.active = viewCodes
		case "8":
			m.active = viewRaw
		case "tab", "right", "l":
			m.active = (m.active + 1) % viewCount
		case "shift+tab", "left", "h":
			m.active = (m.active + viewCount - 1) % viewCount
		case "s":
			m.prompt = &promptState{target: promptSave, buf: defaultBase()}
		case "c":
			m.setNotice(m.clear())
		case "r":
			cmd := m.toggleRecording() // sequence the mutation before reading m
			return m, cmd
		case "d":
			m.toggleCSV()
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
		return m, m.waitForSnapshot()

	case providerDoneMsg:
		m.done = true

	case noticeExpireMsg:
		if int(msg) == m.noticeSeq {
			m.notice = ""
		}
	}
	return m, nil
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
	}
	if !s.ParseOK {
		return
	}
	if ft.ClosedLoop {
		m.intGrid.Add(ft.RPM, ft.MapKPa, s.Sensors["integrator"])
	}
	m.o2Grid.Add(ft.RPM, ft.MapKPa, s.Sensors["oxygen_sensor"]/1000.0)
	// Spark bins per-frame deltas of the cumulative KNOCK_CNT byte (wraps at
	// 255). The first parsed frame only establishes the baseline — WinALDL
	// counts knocks during the session, not the counter's absolute value.
	knock := s.Sensors["knock_count"]
	if m.hasKnockBase {
		if delta := math.Mod(knock-m.knockPrev+256, 256); delta > 0 {
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

// clear resets state for the active tab: the viewed grid (BLM/INT/O2/Spark)
// or, on the sensor tab, the Min/Max extrema. Other tabs are a no-op (notice
// unchanged). Clearing the spark grid keeps the knock baseline — a clear must
// not manufacture a phantom delta on the next frame.
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

// defaultBase is the pre-filled filename prompt value shared by save, record,
// and CSV: a timestamped goaldl_<ts> the operator can accept with one Enter.
func defaultBase() string { return "goaldl_" + time.Now().Format("20060102_150405") }

// handlePromptKey routes keys while the filename prompt is open: printable
// runes (digits, q, spaces, path separators) edit the buffer; enter confirms,
// esc cancels, and only ctrl+c still quits the program.
func (m *tuiModel) handlePromptKey(msg tea.KeyMsg) tea.Cmd {
	p := m.prompt
	switch msg.Type {
	case tea.KeyCtrlC:
		m.cancel()
		return tea.Quit
	case tea.KeyEscape:
		m.prompt = nil
		m.setNotice("cancelled")
	case tea.KeyEnter:
		m.confirmPrompt()
	case tea.KeyBackspace:
		if r := []rune(p.buf); len(r) > 0 {
			p.buf = string(r[:len(r)-1])
		}
		p.hint = ""
	case tea.KeySpace:
		p.buf += " "
		p.hint = ""
	case tea.KeyRunes:
		p.buf += string(msg.Runes)
		p.hint = ""
	}
	return nil
}

// confirmPrompt performs the prompted action with the edited base name. A name
// collision keeps the prompt open (hint set) so the operator can edit and
// retry — files are never silently overwritten.
func (m *tuiModel) confirmPrompt() {
	p := m.prompt
	base := strings.TrimSpace(p.buf)
	if base == "" {
		m.prompt = nil
		m.setNotice("cancelled")
		return
	}
	switch p.target {
	case promptSave:
		// dir "" so the base is used verbatim: bare names land in the working
		// directory, and a typed path (absolute or relative) is honoured.
		if err := saveGrids("", base, m.grid, m.intGrid, m.o2Grid, m.sparkGrid, m.minSamples); err != nil {
			if errors.Is(err, fs.ErrExist) {
				p.hint = "exists — edit the name"
				return
			}
			m.prompt = nil
			m.setNotice("save failed: " + err.Error())
			return
		}
		m.prompt = nil
		m.setNotice("saved BLM/INT/O2/SPARK → " + base + "_*.txt")
	case promptRecord:
		name := base + ".raw"
		f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				p.hint = "exists — edit the name"
				return
			}
			m.prompt = nil
			m.setNotice("record failed: " + err.Error())
			return
		}
		m.recSink.Set(f)
		m.recFile, m.recName = f, name
		m.prompt = nil
		m.setNotice("recording → " + name)
	case promptCSV:
		name := base + ".csv"
		if _, err := os.Stat(name); err == nil {
			p.hint = "exists — edit the name"
			return
		}
		c, err := newFrameCSV(name, m.def)
		if err != nil {
			m.prompt = nil
			m.setNotice("csv failed: " + err.Error())
			return
		}
		m.csvLog, m.csvName = c, name
		m.prompt = nil
		m.setNotice("csv log → " + name)
	}
}

// toggleRecording starts (via the filename prompt) or stops the live raw
// capture. Replay sources have nothing to record — the capture file already
// exists — so `r` warns with a self-expiring notice.
func (m *tuiModel) toggleRecording() tea.Cmd {
	switch {
	case m.recSink == nil:
		return m.warn("recording needs a live source (-p)")
	case m.recFile != nil:
		_, n := m.recSink.Set(nil)
		m.recFile.Close()
		m.setNotice(fmt.Sprintf("stopped recording %s (%s)", m.recName, humanBytes(n)))
		m.recFile, m.recName = nil, ""
	default:
		m.prompt = &promptState{target: promptRecord, buf: defaultBase()}
	}
	return nil
}

// toggleCSV starts (via the filename prompt) or stops the decoded-frame CSV
// log. Works on live and replay sources alike.
func (m *tuiModel) toggleCSV() {
	if m.csvLog != nil {
		rows := m.csvLog.Rows
		m.csvLog.Close()
		m.setNotice(fmt.Sprintf("stopped csv %s (%d rows)", m.csvName, rows))
		m.csvLog, m.csvName = nil, ""
		return
	}
	m.prompt = &promptState{target: promptCSV, buf: defaultBase()}
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
	m.setNotice(fmt.Sprintf("speed %g×", v))
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

// saveGrids writes the BLM, INT, O2, and spark grids to four files in dir
// sharing the caller-chosen base name (`<base>_BLM.txt`, …). Files are created
// exclusively; every target is checked up front so a name collision aborts
// cleanly (overwriting nothing), and a mid-write failure unlinks the files
// already created this call — either way no partial set is left behind.
func saveGrids(dir, base string, blmG, intG, o2G, sparkG *blm.Grid, minSamples int) error {
	files := []struct {
		suffix string
		write  func(io.Writer)
	}{
		{"BLM", func(w io.Writer) { writeTrimGridFile(w, blmG, "BLM", minSamples) }},
		{"INT", func(w io.Writer) { writeTrimGridFile(w, intG, "INT", minSamples) }},
		{"O2", func(w io.Writer) { writeO2File(w, o2G) }},
		{"SPARK", func(w io.Writer) { writeSparkFile(w, sparkG) }},
	}
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
	beatOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	beatBad     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	loopClosed  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green
	loopOpen    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // amber
)

func (m tuiModel) View() string {
	tabs := []string{"1 Sensors", "2 BLM", "3 INT", "4 O2", "5 Spark", "6 Flags", "7 Codes", "8 Raw"}
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
		body = dimStyle.Render("\n  waiting for a parseable frame… (see 8 Raw for the byte stream)")
	case m.active == viewSensors:
		body = stream.SensorTableExtrema(m.lastGood.FrameEvent, m.def, m.mins, m.maxs)
	case m.active == viewBLM:
		body = stream.BLMBodyExplained(m.grid, m.lastGood.FrameEvent, m.minSamples)
	case m.active == viewINT:
		body = stream.INTBody(m.intGrid, m.lastGood.FrameEvent, m.minSamples, m.lastGood.Sensors["integrator"])
	case m.active == viewO2:
		body = stream.O2Body(m.o2Grid, m.lastGood.FrameEvent, m.lastGood.Sensors["oxygen_sensor"]/1000.0)
	case m.active == viewSpark:
		body = stream.SparkBody(m.sparkGrid, m.lastGood.FrameEvent, m.lastGood.Sensors["knock_count"])
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
	footer := status + m.sessionChrome()
	if m.prompt != nil {
		// The prompt replaces the notice/legend segment while it is open.
		footer += "   " + m.promptLine()
	} else {
		if m.notice != "" {
			footer += "   " + m.notice
		}
		footer += "   " + dimStyle.Render("1-8/tab · s save · c clear · r rec · d csv · space/± replay · q quit")
	}

	return header + "\n" + m.loopStatusLine() + "\n\n" + body + "\n\n" + footer
}

// sessionChrome renders the persistent recording/logging/playback segments of
// the footer: a red ● REC while raw capture runs, the CSV row count while
// logging, and the replay pause/speed state.
func (m tuiModel) sessionChrome() string {
	var out string
	if m.recFile != nil {
		out += "   " + beatBad.Render(fmt.Sprintf("● REC %s %s", m.recName, humanBytes(m.recSink.Bytes())))
	}
	if m.csvLog != nil {
		out += "   " + dimStyle.Render(fmt.Sprintf("CSV %s %d", m.csvName, m.csvLog.Rows))
	}
	if m.replay != nil {
		switch sp := m.replay.CurrentSpeed(); {
		case m.replay.Paused():
			out += "   " + loopOpen.Render("⏸ PAUSED")
		case sp != 1 && sp != 0:
			out += "   " + dimStyle.Render(fmt.Sprintf("%g×", sp))
		}
	}
	return out
}

// promptLine renders the open filename prompt with its target-specific
// extension hint (or a transient hint such as a name collision).
func (m tuiModel) promptLine() string {
	p := m.prompt
	label, ext := "save as", "+ _BLM/_INT/_O2/_SPARK.txt"
	switch p.target {
	case promptRecord:
		label, ext = "record to", "+ .raw"
	case promptCSV:
		label, ext = "csv to", "+ .csv"
	}
	hint := ext
	if p.hint != "" {
		hint = p.hint
	}
	return fmt.Sprintf("%s: %s▌  %s", label, p.buf, dimStyle.Render(hint+" · enter confirm · esc cancel"))
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
