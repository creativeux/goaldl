package main

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/stream"
)

func testModel() tuiModel {
	def, _ := ecm.NewRegistry().GetDefinition("1227747")
	return tuiModel{
		def:        def,
		minSamples: blm.DefaultMinSamples,
		source:     "test",
		cancel:     func() {},
		grid:       blm.NewDefault(),
		intGrid:    blm.NewDefault(),
		o2Grid:     blm.NewDefault(),
		sparkGrid:  blm.NewSpark(),
		mins:       map[string]float64{},
		maxs:       map[string]float64{},
	}
}

func runes(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// recordableSnapshot is a closed-loop, block-learn-enabled frame (MWAF1=0x82),
// PROM 24/147, RPM 1600, MAP ~47 kPa, BLM 118 — as a fully processed Snapshot
// (parsed, flags and codes decoded), the way a Session emits it.
func recordableSnapshot() stream.Snapshot {
	def, _ := ecm.NewRegistry().GetDefinition("1227747")
	f := make([]byte, 20)
	f[1], f[2], f[6], f[7], f[14], f[18] = 24, 147, 99, 64, 0x82, 118
	sensors, _ := def.Parse(f)
	return stream.Snapshot{
		FrameEvent: stream.FrameEvent{Frame: decoder.Frame{Data: f}, Index: 0},
		PROMOK:     true,
		ParseOK:    true,
		Sensors:    sensors,
		FuelTrim:   ecm.FuelTrimSample(f),
		Flags:      ecm.DecodeFlags(def, f),
		Codes:      ecm.DecodeCodes(def, f),
	}
}

// tpsAlt returns the TPS-percent alternate conversion's (Factor, Bias) from a
// definition — enough to tell a calibrated def apart from the pristine default
// (AltConversion itself is not comparable: it carries a Lookup).
func tpsAlt(t *testing.T, def *ecm.Definition) (factor, bias float64) {
	t.Helper()
	for _, p := range def.Parameters {
		if p.Name == "tps_voltage" {
			if p.Alt == nil {
				t.Fatal("tps_voltage has no Alt conversion")
			}
			return p.Alt.Factor, p.Alt.Bias
		}
	}
	t.Fatal("no tps_voltage parameter")
	return 0, 0
}

// Regression: flags that trail the capture filename must be honoured. Before
// the two-stage-parse fix, resolveTUIFlags read tps0/tps100/ecmPart *before*
// re-parsing post-filename flags, so `goaldl drive.raw -tps0 …` silently
// applied the defaults (and `-e` desynced the session from the model).
func TestResolveTUIFlagsAfterFilename(t *testing.T) {
	want := ecm.TPSPercentAlt(0.30, 4.50)

	// Flags before the filename already worked; establish the target value.
	before, err := resolveTUIFlags([]string{"-tps0", "0.30", "-tps100", "4.50", "drive.raw"})
	if err != nil {
		t.Fatalf("flags before filename: %v", err)
	}
	if f, b := tpsAlt(t, before.def); f != want.Factor || b != want.Bias {
		t.Fatalf("flags before filename: TPS Alt = (%v, %v), want (%v, %v)", f, b, want.Factor, want.Bias)
	}

	// The regression case: identical flags placed *after* the filename.
	after, err := resolveTUIFlags([]string{"drive.raw", "-tps0", "0.30", "-tps100", "4.50"})
	if err != nil {
		t.Fatalf("flags after filename: %v", err)
	}
	if after.inName != "drive.raw" {
		t.Fatalf("inName = %q, want drive.raw", after.inName)
	}
	if f, b := tpsAlt(t, after.def); f != want.Factor || b != want.Bias {
		t.Fatalf("flags after filename: TPS Alt = (%v, %v), want (%v, %v) (post-filename flag dropped?)", f, b, want.Factor, want.Bias)
	}
}

// The default calibration (no TPS flags) must match the pristine registry def.
func TestResolveTUIFlagsDefaultCalibration(t *testing.T) {
	cfg, err := resolveTUIFlags([]string{"drive.raw"})
	if err != nil {
		t.Fatalf("resolveTUIFlags: %v", err)
	}
	want := ecm.TPSPercentAlt(ecm.DefaultTPS0, ecm.DefaultTPS100)
	if f, b := tpsAlt(t, cfg.def); f != want.Factor || b != want.Bias {
		t.Fatalf("default TPS Alt = (%v, %v), want (%v, %v)", f, b, want.Factor, want.Bias)
	}
}

func TestResolveTUIFlagsNoSource(t *testing.T) {
	if _, err := resolveTUIFlags(nil); !errors.Is(err, errNoTUISource) {
		t.Fatalf("no source: err = %v, want errNoTUISource", err)
	}
}

func TestTUITabSwitching(t *testing.T) {
	m := testModel()
	if m.active != viewSensors {
		t.Fatalf("initial view = %d, want sensors", m.active)
	}
	steps := []struct {
		key  tea.KeyMsg
		want view
	}{
		{runes('2'), viewBLM},
		{runes('3'), viewINT},
		{runes('4'), viewO2},
		{runes('5'), viewSpark},
		{runes('6'), viewFlags},
		{runes('7'), viewCodes},
		{runes('8'), viewRaw},
		{runes('1'), viewSensors},
		{tea.KeyMsg{Type: tea.KeyTab}, viewBLM},     // cycle forward
		{tea.KeyMsg{Type: tea.KeyTab}, viewINT},     //
		{tea.KeyMsg{Type: tea.KeyTab}, viewO2},      //
		{tea.KeyMsg{Type: tea.KeyTab}, viewSpark},   //
		{tea.KeyMsg{Type: tea.KeyShiftTab}, viewO2}, // cycle back
		{runes('1'), viewSensors},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, viewRaw}, // wraps backward
	}
	var cur tea.Model = m
	for i, s := range steps {
		next, _ := cur.Update(s.key)
		if got := next.(tuiModel).active; got != s.want {
			t.Errorf("step %d: active = %d, want %d", i, got, s.want)
		}
		cur = next
	}
}

func TestTUIFrameAccumulates(t *testing.T) {
	m := testModel()
	next, cmd := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	if !mm.hasFrame || mm.frameCount != 1 {
		t.Errorf("hasFrame=%v frameCount=%d, want true/1", mm.hasFrame, mm.frameCount)
	}
	if !mm.hasGood || mm.okCount != 1 || mm.badCount != 0 {
		t.Errorf("hasGood=%v ok=%d bad=%d, want true/1/0", mm.hasGood, mm.okCount, mm.badCount)
	}
	if len(mm.history) != 1 {
		t.Errorf("history holds %d frames, want 1", len(mm.history))
	}
	if mm.grid.TotalSamples() != 1 {
		t.Errorf("grid recorded %d, want 1 (recordable frame)", mm.grid.TotalSamples())
	}
	if cmd == nil {
		t.Error("a snapshot should re-issue the waitForSnapshot command")
	}

	// An open-loop frame (MWAF1=0) advances the frame count but records nothing.
	f := recordableSnapshot()
	f.Frame.Data[14] = 0
	f.FuelTrim = ecm.FuelTrimSample(f.Frame.Data)
	next2, _ := mm.Update(snapshotMsg(f))
	mm2 := next2.(tuiModel)
	if mm2.frameCount != 2 {
		t.Errorf("frameCount = %d, want 2", mm2.frameCount)
	}
	if mm2.grid.TotalSamples() != 1 {
		t.Errorf("grid recorded %d after open-loop frame, want still 1", mm2.grid.TotalSamples())
	}
}

// TestTUIBadFrameGating: a frame that fails to parse still feeds the raw
// history and the bad counter, but the decoded views keep rendering the last
// good frame (WinALDL's bad-sample gating).
func TestTUIBadFrameGating(t *testing.T) {
	m := testModel()
	good := recordableSnapshot()
	next, _ := m.Update(snapshotMsg(good))
	mm := next.(tuiModel)

	bad := stream.Snapshot{
		FrameEvent: stream.FrameEvent{Frame: decoder.Frame{Data: []byte{0xDE, 0xAD}}, Index: 1},
		ParseOK:    false,
	}
	next2, _ := mm.Update(snapshotMsg(bad))
	mm2 := next2.(tuiModel)

	if mm2.badCount != 1 || mm2.okCount != 1 {
		t.Errorf("ok=%d bad=%d, want 1/1", mm2.okCount, mm2.badCount)
	}
	if mm2.lastGood.Index != 0 {
		t.Errorf("lastGood moved to index %d, want 0 (held)", mm2.lastGood.Index)
	}
	if mm2.latest.Index != 1 {
		t.Errorf("latest = %d, want 1 (raw view follows every frame)", mm2.latest.Index)
	}
	if len(mm2.history) != 2 {
		t.Errorf("history holds %d frames, want 2 (bad frames included)", len(mm2.history))
	}
	// The sensors view still renders the good frame's data.
	mm2.active = viewSensors
	if !strings.Contains(mm2.View(), "1600") {
		t.Error("sensors view should hold the last good frame's RPM (1600)")
	}
}

func TestTUIQuit(t *testing.T) {
	m := testModel()
	_, cmd := m.Update(runes('q'))
	if cmd == nil {
		t.Fatal("q should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should quit the program")
	}
}

func TestTUIViewPerTab(t *testing.T) {
	m := testModel()
	if !strings.Contains(m.View(), "waiting for frames") {
		t.Error("view before any frame should show a waiting message")
	}

	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	// Header always shows the tab bar and source; footer the heartbeat counts.
	v := mm.View()
	for _, want := range []string{"Sensors", "BLM", "INT", "O2", "Spark", "Flags", "Codes", "Raw", "test", "PROM ✓", "1 ok / 0 bad"} {
		if !strings.Contains(v, want) {
			t.Errorf("sensors view missing %q", want)
		}
	}
	// The persistent loop-status line shows on every tab (recordable frame → closed).
	if !strings.Contains(v, "CLOSED LOOP") {
		t.Error("view should show the persistent loop-status line")
	}
	if !strings.Contains(v, "Engine speed") {
		t.Error("sensors view should render the sensor table")
	}
	// Sensor tab now carries MIN/MAX columns (always-on).
	for _, want := range []string{"MIN", "MAX"} {
		if !strings.Contains(v, want) {
			t.Errorf("sensors view missing %q column", want)
		}
	}
	// Dual-unit Alt column: MAP byte 99 → (99+28.06)/2.71 ≈ 46.89 kPa.
	if !strings.Contains(v, "46.89 kPa") {
		t.Error("sensors view should render the MAP kPa alternate value")
	}

	mm.active = viewBLM
	if !strings.Contains(mm.View(), "RPM\\MAP") {
		t.Error("BLM view should render the grid")
	}

	mm.active = viewSpark
	sv := mm.View()
	for _, want := range []string{"KNOCK_CNT", "knock events"} {
		if !strings.Contains(sv, want) {
			t.Errorf("spark view missing %q", want)
		}
	}

	// Grid explainers are collapsed by default (compact legend); pressing `i`
	// expands the accordion and the "what this table means" block appears.
	for _, tc := range []struct {
		v      view
		marker string
	}{
		{viewBLM, "Block Learn Multiplier"},
		{viewINT, "Integrator"},
		{viewO2, "stoichiometric"},
		{viewSpark, "detonation"},
	} {
		mm.active = tc.v
		mm.showInfo = false
		if strings.Contains(mm.View(), tc.marker) {
			t.Errorf("tab %d should hide its explainer by default (found %q)", tc.v, tc.marker)
		}
		mm.showInfo = true
		if !strings.Contains(mm.View(), tc.marker) {
			t.Errorf("tab %d should show its explainer with showInfo (want %q)", tc.v, tc.marker)
		}
	}
	mm.showInfo = false

	mm.active = viewFlags
	fv := mm.View()
	for _, want := range []string{"MW2", "MWAF1", "MCU2IO", "Loop status: CLOSED", "Block learn enable"} {
		if !strings.Contains(fv, want) {
			t.Errorf("flags view missing %q", want)
		}
	}

	mm.active = viewCodes
	cv := mm.View()
	for _, want := range []string{"no codes set", "MALFFLG1", "24 — VSS"} {
		if !strings.Contains(cv, want) {
			t.Errorf("codes view missing %q", want)
		}
	}

	mm.active = viewRaw
	rv := mm.View()
	for _, want := range []string{"SAMPLE", "MW2", "MWAF1", "BLM", "118"} {
		if !strings.Contains(rv, want) {
			t.Errorf("raw view missing %q", want)
		}
	}
}

// TestTUIGridAccumulation: a closed-loop frame feeds all three grids and the
// extrema; an open-loop frame still feeds O2 (ungated) but not BLM/INT.
func TestTUIGridAccumulation(t *testing.T) {
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	if mm.grid.TotalSamples() != 1 {
		t.Errorf("BLM grid = %d, want 1", mm.grid.TotalSamples())
	}
	if mm.intGrid.TotalSamples() != 1 {
		t.Errorf("INT grid = %d, want 1 (closed loop)", mm.intGrid.TotalSamples())
	}
	if mm.o2Grid.TotalSamples() != 1 {
		t.Errorf("O2 grid = %d, want 1 (ungated)", mm.o2Grid.TotalSamples())
	}
	if !mm.hasExtrema || len(mm.mins) == 0 {
		t.Error("extrema should be populated after a parseable frame")
	}

	// Open-loop frame (MWAF1=0): O2 keeps accumulating; BLM/INT freeze.
	f := recordableSnapshot()
	f.Frame.Data[14] = 0
	f.FuelTrim = ecm.FuelTrimSample(f.Frame.Data)
	f.Sensors, _ = mm.def.Parse(f.Frame.Data)
	next2, _ := mm.Update(snapshotMsg(f))
	mm2 := next2.(tuiModel)
	if mm2.grid.TotalSamples() != 1 || mm2.intGrid.TotalSamples() != 1 {
		t.Errorf("open loop: BLM=%d INT=%d, want 1/1 (frozen)", mm2.grid.TotalSamples(), mm2.intGrid.TotalSamples())
	}
	if mm2.o2Grid.TotalSamples() != 2 {
		t.Errorf("open loop: O2=%d, want 2 (ungated)", mm2.o2Grid.TotalSamples())
	}
}

// TestTUIClearIsolation: `c` clears only the active grid; on the sensor tab it
// resets the extrema.
func TestTUIClearIsolation(t *testing.T) {
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	// Clear on the INT tab wipes INT only.
	mm.active = viewINT
	next2, _ := mm.Update(runes('c'))
	mm2 := next2.(tuiModel)
	if mm2.intGrid.TotalSamples() != 0 {
		t.Errorf("INT grid = %d after clear, want 0", mm2.intGrid.TotalSamples())
	}
	if mm2.grid.TotalSamples() != 1 || mm2.o2Grid.TotalSamples() != 1 {
		t.Errorf("clear INT should not touch BLM(%d)/O2(%d)", mm2.grid.TotalSamples(), mm2.o2Grid.TotalSamples())
	}
	if !strings.Contains(mm2.notice, "cleared INT") {
		t.Errorf("notice = %q, want cleared INT", mm2.notice)
	}

	// Clear on the sensor tab resets extrema.
	mm2.active = viewSensors
	next3, _ := mm2.Update(runes('c'))
	mm3 := next3.(tuiModel)
	if mm3.hasExtrema || len(mm3.mins) != 0 {
		t.Error("sensor-tab clear should reset extrema")
	}
}

// TestSaveGrids: writes four files under the caller-chosen base; BLM/INT carry
// a correction table, O2 and SPARK do not (voltage / counts).
func TestSaveGrids(t *testing.T) {
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	dir := t.TempDir()
	const base = "session42"
	if err := saveGrids(dir, base, mm.grid, mm.intGrid, mm.o2Grid, mm.sparkGrid, mm.minSamples); err != nil {
		t.Fatalf("saveGrids: %v", err)
	}

	read := func(suffix string) string {
		b, err := os.ReadFile(filepath.Join(dir, base+"_"+suffix+".txt"))
		if err != nil {
			t.Fatalf("read %s: %v", suffix, err)
		}
		return string(b)
	}
	for _, s := range []string{"BLM", "INT"} {
		c := read(s)
		if !strings.Contains(c, "Samples") || !strings.Contains(c, "Wide Average "+s) || !strings.Contains(c, "Correction factor") {
			t.Errorf("%s file missing Samples/Average/Correction:\n%s", s, c)
		}
	}
	o2 := read("O2")
	if !strings.Contains(o2, "Wide Average O2 (volts)") {
		t.Errorf("O2 file missing average:\n%s", o2)
	}
	if strings.Contains(o2, "Correction") {
		t.Errorf("O2 file should have no correction table:\n%s", o2)
	}
	spark := read("SPARK")
	if !strings.Contains(spark, "Samples (frames with knock)") || !strings.Contains(spark, "Knock counts") {
		t.Errorf("SPARK file missing Samples/Knock counts:\n%s", spark)
	}
	if strings.Contains(spark, "Correction") {
		t.Errorf("SPARK file should have no correction table:\n%s", spark)
	}

	// A second save to the same base must refuse (exclusive create), not
	// overwrite.
	if err := saveGrids(dir, base, mm.grid, mm.intGrid, mm.o2Grid, mm.sparkGrid, mm.minSamples); !errors.Is(err, fs.ErrExist) {
		t.Errorf("second save err = %v, want fs.ErrExist", err)
	}
}

// TestTUISparkDeltaAccumulation: the spark grid bins per-frame deltas of the
// cumulative knock counter — first frame is baseline only, unchanged counters
// add nothing, and a wrap (250→4) reads as +10, never a huge negative.
func TestTUISparkDeltaAccumulation(t *testing.T) {
	knockSnap := func(knock byte) stream.Snapshot {
		s := recordableSnapshot()
		s.Frame.Data[17] = knock
		s.Sensors, _ = testModel().def.Parse(s.Frame.Data)
		return s
	}
	sparkTotal := func(g *blm.Grid) float64 {
		var n float64
		for _, row := range g.Sum() {
			for _, v := range row {
				n += v
			}
		}
		return n
	}

	var cur tea.Model = testModel()
	for _, k := range []byte{10, 12, 12} { // baseline, +2, +0
		cur, _ = cur.Update(snapshotMsg(knockSnap(k)))
	}
	mm := cur.(tuiModel)
	if got := sparkTotal(mm.sparkGrid); got != 2 {
		t.Errorf("spark total = %v, want 2 (baseline + delta 2 + delta 0)", got)
	}
	if got := mm.sparkGrid.TotalSamples(); got != 1 {
		t.Errorf("spark samples = %d, want 1 (one frame with knock)", got)
	}

	// Counter wrap: 250 → 4 is a delta of 10 (mod 256).
	var cur2 tea.Model = testModel()
	for _, k := range []byte{250, 4} {
		cur2, _ = cur2.Update(snapshotMsg(knockSnap(k)))
	}
	if got := sparkTotal(cur2.(tuiModel).sparkGrid); got != 10 {
		t.Errorf("wrapped spark total = %v, want 10", got)
	}

	// Clear keeps the knock baseline: the next unchanged counter adds nothing.
	mm.active = viewSpark
	next, _ := mm.Update(runes('c'))
	mm2 := next.(tuiModel)
	if got := sparkTotal(mm2.sparkGrid); got != 0 {
		t.Errorf("cleared spark total = %v, want 0", got)
	}
	next2, _ := mm2.Update(snapshotMsg(knockSnap(12)))
	if got := sparkTotal(next2.(tuiModel).sparkGrid); got != 0 {
		t.Errorf("post-clear unchanged counter added %v, want 0 (baseline kept)", got)
	}
}

// TestTUIPromptEditing: while the prompt is open, digits and q type into the
// buffer (no tab switch, no quit); esc cancels without writing; enter writes
// the four grid files under the edited base; an existing file keeps the
// prompt open instead of overwriting.
func TestTUIPromptEditing(t *testing.T) {
	dir := t.TempDir()
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	// `s` opens the save prompt pre-filled with the timestamped default.
	next2, _ := mm.Update(runes('s'))
	mm2 := next2.(tuiModel)
	if mm2.prompt == nil || mm2.prompt.target != promptSave {
		t.Fatal("s should open the save prompt")
	}
	if !strings.HasPrefix(mm2.prompt.buf, "goaldl_") {
		t.Errorf("prompt default = %q, want goaldl_<ts>", mm2.prompt.buf)
	}
	if !strings.Contains(mm2.View(), "save as:") {
		t.Error("view should render the open prompt")
	}

	// Keys route to the buffer: digits don't switch tabs, q doesn't quit.
	before := mm2.active
	var cur tea.Model = mm2
	for _, r := range "2q" {
		var cmd tea.Cmd
		cur, cmd = cur.Update(runes(r))
		if cmd != nil {
			t.Fatalf("typing %q returned a command (quit?)", r)
		}
	}
	mm3 := cur.(tuiModel)
	if mm3.active != before {
		t.Error("digit typed into the prompt switched tabs")
	}
	if !strings.HasSuffix(mm3.prompt.buf, "2q") {
		t.Errorf("buffer = %q, want …2q appended", mm3.prompt.buf)
	}

	// Esc cancels without writing anything.
	next4, _ := mm3.Update(tea.KeyMsg{Type: tea.KeyEscape})
	mm4 := next4.(tuiModel)
	if mm4.prompt != nil {
		t.Fatal("esc should close the prompt")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("esc wrote %d files, want 0", len(ents))
	}

	// Re-open, replace the buffer with a temp-dir base, confirm: 4 files.
	next5, _ := mm4.Update(runes('s'))
	mm5 := next5.(tuiModel)
	mm5.prompt.buf = filepath.Join(dir, "mysession")
	next6, _ := mm5.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm6 := next6.(tuiModel)
	if mm6.prompt != nil {
		t.Fatalf("enter should close the prompt (hint %q)", mm6.prompt.hint)
	}
	if !strings.Contains(mm6.notice, "mysession") {
		t.Errorf("notice = %q, want the saved base", mm6.notice)
	}
	for _, s := range []string{"BLM", "INT", "O2", "SPARK"} {
		if _, err := os.Stat(filepath.Join(dir, "mysession_"+s+".txt")); err != nil {
			t.Errorf("missing %s file: %v", s, err)
		}
	}

	// Confirming the same base again collides: prompt stays open with a hint.
	next7, _ := mm6.Update(runes('s'))
	mm7 := next7.(tuiModel)
	mm7.prompt.buf = filepath.Join(dir, "mysession")
	next8, _ := mm7.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm8 := next8.(tuiModel)
	if mm8.prompt == nil {
		t.Fatal("collision should keep the prompt open")
	}
	if mm8.prompt.hint == "" {
		t.Error("collision should set the prompt hint")
	}
}

// TestTUINoticeExpiry: a no-op warning (e.g. `r` during replay) arms a timer
// and clears itself when it fires; a stale timer never wipes a newer notice.
func TestTUINoticeExpiry(t *testing.T) {
	m := testModel() // replay-style model: no RecordSink
	next, cmd := m.Update(runes('r'))
	mm := next.(tuiModel)
	if !strings.Contains(mm.notice, "live source") {
		t.Fatalf("notice = %q, want live-source warning", mm.notice)
	}
	if cmd == nil {
		t.Fatal("warning should arm an expiry timer")
	}
	seq := mm.noticeSeq

	// The armed timer fires → the warning clears.
	next2, _ := mm.Update(noticeExpireMsg(seq))
	if got := next2.(tuiModel).notice; got != "" {
		t.Errorf("notice after expiry = %q, want cleared", got)
	}

	// A newer notice must survive the old warning's stale timer.
	mm.active = viewBLM
	next3, _ := mm.Update(runes('c')) // persistent "cleared BLM grid"
	mm3 := next3.(tuiModel)
	next4, _ := mm3.Update(noticeExpireMsg(seq)) // stale seq
	if got := next4.(tuiModel).notice; !strings.Contains(got, "cleared BLM") {
		t.Errorf("stale expiry wiped a newer notice: %q", got)
	}
}

// TestTUIRecordingToggle: `r` is a notice-only no-op on replay (no RecordSink);
// with a live sink it prompts, records through the sink, and stops with a
// byte-count notice.
func TestTUIRecordingToggle(t *testing.T) {
	// Replay source: no sink.
	m := testModel()
	next, _ := m.Update(runes('r'))
	mm := next.(tuiModel)
	if mm.prompt != nil {
		t.Fatal("r on replay must not open a prompt")
	}
	if !strings.Contains(mm.notice, "live source") {
		t.Errorf("notice = %q, want live-source explanation", mm.notice)
	}

	// Live source: sink present.
	dir := t.TempDir()
	live := testModel()
	live.recSink = &stream.RecordSink{}
	next2, _ := live.Update(runes('r'))
	mm2 := next2.(tuiModel)
	if mm2.prompt == nil || mm2.prompt.target != promptRecord {
		t.Fatal("r with a sink should open the record prompt")
	}
	mm2.prompt.buf = filepath.Join(dir, "capture")
	next3, _ := mm2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm3 := next3.(tuiModel)
	if mm3.recFile == nil || !mm3.recSink.Active() {
		t.Fatal("confirm should attach the recording target")
	}
	if !strings.Contains(mm3.notice, "recording → ") {
		t.Errorf("notice = %q, want recording start", mm3.notice)
	}
	// The provider's writes flow through the sink into the file.
	mm3.recSink.Write([]byte{0xFE, 0x00, 0xFE})

	next4, _ := mm3.Update(runes('r'))
	mm4 := next4.(tuiModel)
	if mm4.recFile != nil || mm4.recSink.Active() {
		t.Fatal("second r should stop the recording")
	}
	if !strings.Contains(mm4.notice, "stopped recording") || !strings.Contains(mm4.notice, "3 B") {
		t.Errorf("notice = %q, want stopped + 3 B", mm4.notice)
	}
	b, err := os.ReadFile(filepath.Join(dir, "capture.raw"))
	if err != nil || len(b) != 3 {
		t.Errorf("capture.raw = %d bytes (%v), want 3", len(b), err)
	}
}

// TestTUICSVToggle: `d` starts a CSV log that writes one row per ParseOK frame
// (bad frames skipped — monitor parity) and stops with a row-count notice.
func TestTUICSVToggle(t *testing.T) {
	dir := t.TempDir()
	m := testModel()
	next, _ := m.Update(runes('d'))
	mm := next.(tuiModel)
	if mm.prompt == nil || mm.prompt.target != promptCSV {
		t.Fatal("d should open the csv prompt")
	}
	mm.prompt.buf = filepath.Join(dir, "log")
	next2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm2 := next2.(tuiModel)
	if mm2.csvLog == nil {
		t.Fatal("confirm should open the csv log")
	}

	var cur tea.Model = mm2
	cur, _ = cur.Update(snapshotMsg(recordableSnapshot()))
	bad := stream.Snapshot{
		FrameEvent: stream.FrameEvent{Frame: decoder.Frame{Data: []byte{0xDE, 0xAD}}, Index: 1},
		ParseOK:    false,
	}
	cur, _ = cur.Update(snapshotMsg(bad))
	cur, _ = cur.Update(snapshotMsg(recordableSnapshot()))
	mm3 := cur.(tuiModel)
	if mm3.csvLog.Rows != 2 {
		t.Errorf("csv rows = %d, want 2 (ParseOK frames only)", mm3.csvLog.Rows)
	}

	next4, _ := mm3.Update(runes('d'))
	mm4 := next4.(tuiModel)
	if mm4.csvLog != nil {
		t.Fatal("second d should stop the log")
	}
	if !strings.Contains(mm4.notice, "2 rows") {
		t.Errorf("notice = %q, want 2 rows", mm4.notice)
	}
	b, err := os.ReadFile(filepath.Join(dir, "log.csv"))
	if err != nil {
		t.Fatalf("read log.csv: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Errorf("csv has %d lines, want 3 (header + 2 rows)", len(lines))
	}
	if !strings.HasPrefix(lines[0], "time_sec,byte_offset,prom_ok") {
		t.Errorf("csv header = %q", lines[0])
	}
}

// TestTUICloseOutputs: quitting with an active recording and CSV log detaches
// the sink before closing the file (so the provider goroutine cannot write to
// a closed handle) and closes both outputs.
func TestTUICloseOutputs(t *testing.T) {
	dir := t.TempDir()
	m := testModel()
	m.recSink = &stream.RecordSink{}

	f, err := os.Create(filepath.Join(dir, "quit.raw"))
	if err != nil {
		t.Fatal(err)
	}
	m.recSink.Set(f)
	m.recFile, m.recName = f, "quit.raw"
	c, err := newFrameCSV(filepath.Join(dir, "quit.csv"), m.def)
	if err != nil {
		t.Fatal(err)
	}
	m.csvLog = c

	m.closeOutputs()

	if m.recSink.Active() {
		t.Error("closeOutputs should detach the sink before closing the file")
	}
	// Both handles are closed: further writes/closes must fail.
	if _, err := f.Write([]byte{0}); err == nil {
		t.Error("recording file still writable after closeOutputs")
	}
	if err := c.Close(); err == nil {
		t.Error("csv file still open after closeOutputs")
	}
	// A provider write after quit is safely discarded, not a panic/write-to-closed.
	if n, err := m.recSink.Write([]byte{0xFE}); n != 1 || err != nil {
		t.Errorf("post-quit sink write = (%d, %v), want discarded (1, nil)", n, err)
	}
}

// TestTUIReplayKeys: space/+/- act on the replay provider (clamped), and are
// notice-only no-ops on a live source or an unpaced (-speed 0) replay.
func TestTUIReplayKeys(t *testing.T) {
	// Live source: no replay handle.
	m := testModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !strings.Contains(next.(tuiModel).notice, "replay-only") {
		t.Errorf("live space notice = %q, want replay-only", next.(tuiModel).notice)
	}

	// Unpaced replay: controls inert.
	un := testModel()
	un.replay = &stream.ReplayProvider{Speed: 0}
	next2, _ := un.Update(runes('+'))
	if !strings.Contains(next2.(tuiModel).notice, "unpaced") {
		t.Errorf("unpaced + notice = %q, want unpaced explanation", next2.(tuiModel).notice)
	}

	// Paced replay: pause toggles, speed doubles/halves with clamping.
	r := testModel()
	r.replay = &stream.ReplayProvider{Speed: 1.0}
	var cur tea.Model = r
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !r.replay.Paused() {
		t.Error("space should pause the replay")
	}
	if !strings.Contains(cur.(tuiModel).View(), "PAUSED") {
		t.Error("footer should show the paused badge")
	}
	cur, _ = cur.Update(tea.KeyMsg{Type: tea.KeySpace})
	if r.replay.Paused() {
		t.Error("space again should resume")
	}
	cur, _ = cur.Update(runes('+'))
	if got := r.replay.CurrentSpeed(); got != 2 {
		t.Errorf("speed after + = %v, want 2", got)
	}
	if mm := cur.(tuiModel); !strings.Contains(mm.View(), "2×") {
		t.Error("footer should show the 2× speed")
	}
	for range 10 {
		cur, _ = cur.Update(runes('+'))
	}
	if got := r.replay.CurrentSpeed(); got != 16 {
		t.Errorf("speed clamps at %v, want 16", got)
	}
	for range 20 {
		cur, _ = cur.Update(runes('-'))
	}
	if got := r.replay.CurrentSpeed(); got != 0.25 {
		t.Errorf("speed clamps at %v, want 0.25", got)
	}
	_ = cur
}

// TestTUIDriveFixtureEndToEnd drives the full real drive capture through a
// Session into the dashboard model, exercising the accumulation path over every
// frame. The BLM count must match the `blm` command's 469 (shared gating),
// INT (closed-loop only) must exceed it, O2 (ungated) must exceed INT, the
// spark total must match an independent recomputation of the knock-counter
// deltas, and every tab must render without panic.
func TestTUIDriveFixtureEndToEnd(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "pkg", "decoder", "testdata", "drive_4800.raw"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	registry := ecm.NewRegistry()
	provider := &stream.ReplayProvider{Data: raw, Config: decoder.DefaultConfig(), Speed: 0}
	session := stream.NewSession(provider, registry, "1227747", 6291)

	// Independent spark oracle: sum the mod-256 deltas of the parsed
	// knock_count across ParseOK frames, first frame as baseline.
	var wantSpark float64
	var prevKnock float64
	haveBase := false

	var cur tea.Model = testModel()
	if err := session.Run(t.Context(), func(s stream.Snapshot) {
		if s.ParseOK {
			k := s.Sensors["knock_count"]
			if haveBase {
				wantSpark += math.Mod(k-prevKnock+256, 256)
			}
			prevKnock, haveBase = k, true
		}
		cur, _ = cur.Update(snapshotMsg(s))
	}); err != nil {
		t.Fatalf("session run: %v", err)
	}
	mm := cur.(tuiModel)

	var gotSpark float64
	for _, row := range mm.sparkGrid.Sum() {
		for _, v := range row {
			gotSpark += v
		}
	}
	if gotSpark != wantSpark {
		t.Errorf("spark total = %v, want %v (independent delta recomputation)", gotSpark, wantSpark)
	}

	if mm.grid.TotalSamples() != 469 {
		t.Errorf("BLM grid = %d, want 469 (matches the blm command)", mm.grid.TotalSamples())
	}
	if mm.intGrid.TotalSamples() <= mm.grid.TotalSamples() {
		t.Errorf("INT grid = %d, want > BLM %d (closed-loop is a looser gate)", mm.intGrid.TotalSamples(), mm.grid.TotalSamples())
	}
	if mm.o2Grid.TotalSamples() < mm.intGrid.TotalSamples() {
		t.Errorf("O2 grid = %d, want >= INT %d (ungated)", mm.o2Grid.TotalSamples(), mm.intGrid.TotalSamples())
	}
	if !mm.hasExtrema {
		t.Error("extrema should be populated after the drive")
	}

	// Every tab renders (loop line is always present).
	for v := view(0); v < viewCount; v++ {
		mm.active = v
		out := mm.View()
		if !strings.Contains(out, "LOOP") {
			t.Errorf("tab %d missing the persistent loop-status line", v)
		}
	}
}

// TestTUILoopLineHoldsLastGood: the persistent loop line reflects the last
// parseable frame and does not flicker on a following bad frame.
func TestTUILoopLineHoldsLastGood(t *testing.T) {
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot())) // closed loop
	mm := next.(tuiModel)
	if !strings.Contains(mm.loopStatusLine(), "CLOSED LOOP") {
		t.Errorf("loop line = %q, want CLOSED LOOP", mm.loopStatusLine())
	}

	bad := stream.Snapshot{
		FrameEvent: stream.FrameEvent{Frame: decoder.Frame{Data: []byte{0xDE, 0xAD}}, Index: 1},
		ParseOK:    false,
	}
	next2, _ := mm.Update(snapshotMsg(bad))
	mm2 := next2.(tuiModel)
	if !strings.Contains(mm2.loopStatusLine(), "CLOSED LOOP") {
		t.Errorf("after bad frame loop line = %q, want held CLOSED LOOP", mm2.loopStatusLine())
	}
}

// TestTUIInfoAccordion: `i` toggles the grid explainer; it is collapsed by
// default and expands to the full explainer, and toggling re-homes the scroll.
func TestTUIInfoAccordion(t *testing.T) {
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)
	mm.active = viewBLM

	if mm.showInfo {
		t.Fatal("explainer should be collapsed by default")
	}
	if strings.Contains(mm.View(), "Block Learn Multiplier") {
		t.Error("collapsed BLM tab should not show the explainer")
	}

	mm.scroll = 3
	toggled, _ := mm.Update(runes('i'))
	tm := toggled.(tuiModel)
	if !tm.showInfo {
		t.Error("i should expand the explainer")
	}
	if tm.scroll != 0 {
		t.Errorf("toggling the accordion should re-home scroll, got %d", tm.scroll)
	}
	if !strings.Contains(tm.View(), "Block Learn Multiplier") {
		t.Error("expanded BLM tab should show the explainer")
	}
}

// TestTUIBodyScroll: when the body is taller than the terminal, the frame is
// clamped to the height (tabs never scroll off), the body window scrolls with
// j/k, and switching tabs re-homes the scroll.
func TestTUIBodyScroll(t *testing.T) {
	m := testModel()
	m.width, m.height = 80, 12 // deliberately short
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)
	mm.active = viewRaw // the tallest body (one row per frame byte)

	// The whole frame fits the terminal height — the tab bar is never pushed off.
	if lines := strings.Count(mm.View(), "\n") + 1; lines > mm.height {
		t.Errorf("frame is %d lines, exceeds height %d (tabs would scroll off)", lines, mm.height)
	}
	if !strings.Contains(mm.View(), "Sensors") {
		t.Error("the tab bar must stay visible on a short terminal")
	}
	if !strings.Contains(mm.View(), "scroll") {
		t.Error("an overflowing body should show the scroll status line")
	}

	// j scrolls down; the position indicator advances.
	down, _ := mm.Update(runes('j'))
	dm := down.(tuiModel)
	if dm.scroll != 1 {
		t.Errorf("j should scroll down one line, got %d", dm.scroll)
	}
	// k cannot scroll above the top.
	up, _ := mm.Update(runes('k'))
	if s := up.(tuiModel).scroll; s != 0 {
		t.Errorf("k at the top should stay at 0, got %d", s)
	}
	// scroll is clamped to maxScroll — hammering j never runs past the end.
	hammered := dm
	for i := 0; i < 500; i++ {
		h, _ := hammered.Update(runes('j'))
		hammered = h.(tuiModel)
	}
	if hammered.scroll != hammered.maxScroll() {
		t.Errorf("scroll %d should be clamped to maxScroll %d", hammered.scroll, hammered.maxScroll())
	}

	// Switching tabs re-homes the scroll.
	switched, _ := hammered.Update(runes('1'))
	if s := switched.(tuiModel).scroll; s != 0 {
		t.Errorf("switching tabs should reset scroll, got %d", s)
	}
}

// TestTUIWidthFit: on a narrow terminal every line of the frame fits the width
// (no soft-wrap), so the pinned tab bar can't be pushed off by a wrapped chrome
// line — including the long footer key legend and the wide Spark grid.
func TestTUIWidthFit(t *testing.T) {
	m := testModel()
	m.width, m.height = 44, 24
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	for _, tab := range []view{viewSensors, viewSpark, viewRaw, viewBLM} {
		mm.active = tab
		v := mm.View()
		if !strings.Contains(v, "Sensors") {
			t.Errorf("tab %d: tab bar must stay visible", tab)
		}
		for _, ln := range strings.Split(v, "\n") {
			if w := ansi.StringWidth(ln); w > mm.width {
				t.Errorf("tab %d: line exceeds width %d (%d): %q", tab, mm.width, w, ln)
			}
		}
	}
}

// knockSnapshot is a parseable closed-loop frame carrying a specific KNOCK_CNT
// byte — for driving the free-running-counter detector with crafted deltas.
func knockSnapshot(knock byte) stream.Snapshot {
	def, _ := ecm.NewRegistry().GetDefinition("1227747")
	f := make([]byte, 20)
	f[1], f[2], f[6], f[7], f[14], f[17], f[18] = 24, 147, 99, 64, 0x82, knock, 118
	sensors, _ := def.Parse(f)
	return stream.Snapshot{
		FrameEvent: stream.FrameEvent{Frame: decoder.Frame{Data: f}, Index: 0},
		PROMOK:     true,
		ParseOK:    true,
		Sensors:    sensors,
		FuelTrim:   ecm.FuelTrimSample(f),
	}
}

// TestTUIFatalError: a transport error ends the stream with a diagnosis panel;
// a clean end or the user's own cancellation is not an error.
func TestTUIFatalError(t *testing.T) {
	// Live source (replay == nil): fatal error → panel with the error text and
	// the serial hints.
	m := testModel()
	next, _ := m.Update(providerDoneMsg{err: errors.New("serial: open /dev/cu.usbserial-10: no such file")})
	mm := next.(tuiModel)
	if mm.fatalErr == nil {
		t.Fatal("a transport error should set fatalErr")
	}
	out := mm.View()
	for _, want := range []string{"Cannot read from", "no such file", "goaldl ports", "-invert"} {
		if !strings.Contains(out, want) {
			t.Errorf("error panel missing %q:\n%s", want, out)
		}
	}

	// Replay source: same fatal path, but no serial hints (the file, not a port).
	r := testModel()
	r.replay = &stream.ReplayProvider{}
	nr, _ := r.Update(providerDoneMsg{err: errors.New("decode failed")})
	if out := nr.(tuiModel).View(); strings.Contains(out, "goaldl ports") {
		t.Errorf("replay error panel should omit serial hints:\n%s", out)
	}

	// User quit (context.Canceled): stream done, but not an error.
	c := testModel()
	nc, _ := c.Update(providerDoneMsg{err: context.Canceled})
	if mc := nc.(tuiModel); mc.fatalErr != nil || !mc.done {
		t.Errorf("cancellation: fatalErr=%v done=%v, want nil/true", mc.fatalErr, mc.done)
	}

	// Clean end (nil): the existing "(stream ended)" path, no panel.
	e := testModel()
	ne, _ := e.Update(providerDoneMsg{err: nil})
	if me := ne.(tuiModel); me.fatalErr != nil {
		t.Errorf("clean end should not set fatalErr, got %v", me.fatalErr)
	}
}

// TestTUIStale: a live stream that goes quiet past staleAfter is flagged (hollow
// heartbeat + footer age); replay, pre-frame, and ended streams are never stale,
// and a fresh frame clears it.
func TestTUIStale(t *testing.T) {
	// Live model with one frame delivered.
	base := testModel()
	next, _ := base.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	// 6.1s since the last frame → stale.
	mm.now = mm.lastFrameAt.Add(6100 * time.Millisecond)
	if stale, _ := mm.stale(); !stale {
		t.Error("6.1s of silence on a live source should be stale")
	}
	if !strings.Contains(mm.heartbeat(), "○") {
		t.Errorf("stale heartbeat should be the hollow glyph, got %q", mm.heartbeat())
	}
	if !strings.Contains(mm.View(), "no data") {
		t.Error("stale view footer should show 'no data'")
	}

	// 2s → still fresh.
	mm.now = mm.lastFrameAt.Add(2 * time.Second)
	if stale, _ := mm.stale(); stale {
		t.Error("2s of silence should not be stale")
	}

	// Replay is never stale, however long.
	rep := testModel()
	rep.replay = &stream.ReplayProvider{}
	nr, _ := rep.Update(snapshotMsg(recordableSnapshot()))
	rm := nr.(tuiModel)
	rm.now = rm.lastFrameAt.Add(30 * time.Second)
	if stale, _ := rm.stale(); stale {
		t.Error("a replay source should never be flagged stale")
	}

	// An ended stream reports (stream ended), not stale.
	mm.done = true
	mm.now = mm.lastFrameAt.Add(30 * time.Second)
	if stale, _ := mm.stale(); stale {
		t.Error("an ended stream should not be stale")
	}

	// Recovery: a new frame resets the clock.
	mm.done = false
	rec, _ := mm.Update(snapshotMsg(recordableSnapshot()))
	if stale, _ := rec.(tuiModel).stale(); stale {
		t.Error("a fresh frame should clear the stale flag")
	}
}

// TestTUIKnockFreeRunning: the drive fixture's free-running KNOCK_CNT trips the
// detector (and the Spark warning); a crafted sparse-knock stream does not; and
// clearing the Spark grid preserves the detection window.
func TestTUIKnockFreeRunning(t *testing.T) {
	// Drive fixture: the counter advances nearly every frame.
	raw, err := os.ReadFile(filepath.Join("..", "..", "pkg", "decoder", "testdata", "drive_4800.raw"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	provider := &stream.ReplayProvider{Data: raw, Config: decoder.DefaultConfig(), Speed: 0}
	session := stream.NewSession(provider, ecm.NewRegistry(), "1227747", 6291)
	var cur tea.Model = testModel()
	if err := session.Run(context.Background(), func(s stream.Snapshot) {
		cur, _ = cur.Update(snapshotMsg(s))
	}); err != nil {
		t.Fatalf("session run: %v", err)
	}
	mm := cur.(tuiModel)
	if !mm.knockFreeRunning() {
		t.Error("drive fixture's free-running KNOCK_CNT should be detected")
	}
	mm.active = viewSpark
	if !strings.Contains(mm.View(), "free-running counter — not knock") {
		t.Error("Spark tab should warn when the counter is free-running")
	}

	// Crafted sparse knock: nonzero delta only every 10th frame → well under the
	// threshold, so no warning.
	sparse := testModel()
	var sm tea.Model = sparse
	for i := 0; i < 40; i++ {
		sm, _ = sm.Update(snapshotMsg(knockSnapshot(byte((i / 10) * 5)))) // 0,0,…,5,…,10,…
	}
	if sm.(tuiModel).knockFreeRunning() {
		t.Error("sparse knock (10% of frames) should not read as free-running")
	}

	// Clearing the Spark grid keeps the detection window (a fact about the
	// counter, not the grid) and the knock baseline.
	mm.active = viewSpark
	cleared, _ := mm.Update(runes('c'))
	cm := cleared.(tuiModel)
	if !cm.knockFreeRunning() {
		t.Error("clearing the Spark grid must preserve the free-running detection")
	}
	var sum float64
	for _, row := range cm.sparkGrid.Sum() {
		for _, v := range row {
			sum += v
		}
	}
	if sum != 0 {
		t.Errorf("cleared spark grid should be empty, sum = %v", sum)
	}
}
