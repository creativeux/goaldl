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
		buf:        newFrameBuf(),
		mins:       map[string]float64{},
		maxs:       map[string]float64{},
	}
}

func runes(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// key helpers for driving the output picker in tests.
var (
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keySpace = tea.KeyMsg{Type: tea.KeySpace}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEscape}
)

// typeInto feeds each rune of s into the model as a KeyRunes message.
func typeInto(m tea.Model, s string) tea.Model {
	for _, r := range s {
		m, _ = m.Update(runes(r))
	}
	return m
}

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

	// Title bar shows the brand + the Signal indicator; the grid tabs carry the
	// per-grid accumulation dots.
	v := mm.View()
	for _, want := range []string{"Sensors", "BLM", "INT", "O2", "Spark", "Flags", "Codes", "Raw", "GoALDL", "Signal:"} {
		if !strings.Contains(v, want) {
			t.Errorf("sensors view missing %q", want)
		}
	}
	// The per-grid accumulation dots are on the grid tabs themselves (recordable
	// frame → closed loop → BLM ●). (The loop word itself is not in the title bar.)
	if !strings.Contains(v, "BLM ●") {
		t.Error("BLM tab should carry a filled accumulation dot on a closed-loop frame")
	}
	if strings.Contains(v, "PROM ✓") {
		t.Error("PROM mark should no longer be in the sensor-tab chrome (replaced by the loop badge)")
	}
	if !strings.Contains(v, "Engine speed") {
		t.Error("sensors view should render the sensor table")
	}
	// MIN/MAX columns are hidden for now (focus on VALUE/ALT).
	for _, gone := range []string{"MIN", "MAX"} {
		if strings.Contains(v, gone) {
			t.Errorf("sensors view should not render the %q column (hidden for now)", gone)
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
	for _, want := range []string{"RPM\\MAP", "knock events"} {
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
	sels := allGridSels(mm.grid, mm.intGrid, mm.o2Grid, mm.sparkGrid, mm.minSamples)
	if err := saveGrids(dir, base, sels); err != nil {
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
	if err := saveGrids(dir, base, sels); !errors.Is(err, fs.ErrExist) {
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

// TestTUISaveBufferPicker: `s` opens the Save Buffer checklist (all outputs on,
// no RAW). While it's open, q doesn't quit; esc cancels without writing; a
// confirm writes the selected files under the edited name; re-confirming the
// same name collides and keeps the picker open with a hint.
func TestTUISaveBufferPicker(t *testing.T) {
	dir := t.TempDir()
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	// `s` opens the Save Buffer picker: 5 outputs (csv + 4 grids), all on, no RAW.
	next2, _ := mm.Update(runes('s'))
	mm2 := next2.(tuiModel)
	if mm2.picker == nil || mm2.picker.op != opSaveBuffer {
		t.Fatal("s should open the Save Buffer picker")
	}
	if len(mm2.picker.items) != 5 {
		t.Errorf("picker has %d items, want 5", len(mm2.picker.items))
	}
	for _, it := range mm2.picker.items {
		if it.id == "raw" {
			t.Error("Save Buffer must not offer RAW")
		}
	}
	if !strings.HasPrefix(mm2.picker.name, "goaldl_") {
		t.Errorf("default name = %q, want goaldl_<ts>", mm2.picker.name)
	}
	if mm2.picker.dir == "" {
		t.Error("dir field should default to a working directory")
	}
	if !strings.Contains(mm2.View(), "Save Buffer") {
		t.Error("view should render the open picker")
	}

	// q while the picker is open must not quit.
	if _, cmd := mm2.Update(runes('q')); cmd != nil {
		t.Fatal("q inside the picker returned a command (quit?)")
	}

	// Both path fields edit via keys: ↓ to the dir row then the name row, each
	// accepting typed runes independently.
	dirRow, nameRow := mm2.picker.dirRow(), mm2.picker.nameRow()
	var cur tea.Model = mm2
	for i := 0; i < dirRow; i++ {
		cur, _ = cur.Update(keyDown)
	}
	cur = typeInto(cur, "X")
	if p := cur.(tuiModel).picker; !strings.HasSuffix(p.dir, "X") {
		t.Errorf("dir after typing = %q, want …X", p.dir)
	}
	cur, _ = cur.Update(keyDown) // to the name row
	if cur.(tuiModel).picker.cursor != nameRow {
		t.Fatalf("cursor = %d, want name row %d", cur.(tuiModel).picker.cursor, nameRow)
	}
	cur = typeInto(cur, "Y")
	if p := cur.(tuiModel).picker; !strings.HasSuffix(p.name, "Y") {
		t.Errorf("name after typing = %q, want …Y", p.name)
	}

	// Esc cancels without writing anything.
	next3, _ := cur.Update(keyEsc)
	if next3.(tuiModel).picker != nil {
		t.Fatal("esc should close the picker")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("esc wrote %d files, want 0", len(ents))
	}

	// Re-open, set the dir + name fields separately, confirm: 4 grid files + CSV.
	next4, _ := next3.(tuiModel).Update(runes('s'))
	mm4 := next4.(tuiModel)
	mm4.picker.dir, mm4.picker.name = dir, "mysession"
	next5, _ := mm4.Update(keyEnter)
	mm5 := next5.(tuiModel)
	if mm5.picker != nil {
		t.Fatalf("enter should close the picker (hint %q)", mm4.picker.hint)
	}
	if !strings.Contains(mm5.notice, "mysession") {
		t.Errorf("notice = %q, want the saved base", mm5.notice)
	}
	for _, s := range []string{"BLM", "INT", "O2", "SPARK"} {
		if _, err := os.Stat(filepath.Join(dir, "mysession_"+s+".txt")); err != nil {
			t.Errorf("missing %s file: %v", s, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "mysession.csv")); err != nil {
		t.Errorf("missing csv: %v", err)
	}

	// Confirming the same dir+name again collides: picker stays open with a hint.
	next6, _ := mm5.Update(runes('s'))
	mm6 := next6.(tuiModel)
	mm6.picker.dir, mm6.picker.name = dir, "mysession"
	next7, _ := mm6.Update(keyEnter)
	mm7 := next7.(tuiModel)
	if mm7.picker == nil {
		t.Fatal("collision should keep the picker open")
	}
	if mm7.picker.hint == "" {
		t.Error("collision should set the picker hint")
	}
}

// TestTUINoticeExpiry: a no-op warning (e.g. pause/speed with no replay) arms a
// timer and clears itself when it fires; a stale timer never wipes a newer notice.
func TestTUINoticeExpiry(t *testing.T) {
	m := testModel() // no replay handle: space is a no-op warning
	next, cmd := m.Update(keySpace)
	mm := next.(tuiModel)
	if !strings.Contains(mm.notice, "replay-only") {
		t.Fatalf("notice = %q, want replay-only warning", mm.notice)
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

// TestLogForward: `r` opens the Log picker (forward streaming). RAW is offered
// only when a live sink is present. Confirming RAW+CSV attaches the sink and
// opens the CSV; new frames flow to both; a second `r` stops both, records them
// for the exit summary, and leaves usable files.
func TestLogForward(t *testing.T) {
	// Replay-style model (no sink): the Log picker lists RAW but disables it
	// (visible, not hidden) so its selection is a no-op.
	noSink := testModel()
	np, _ := noSink.Update(runes('l'))
	npm := np.(tuiModel)
	if npm.picker == nil || npm.picker.op != opLog {
		t.Fatal("l should open the Log picker")
	}
	var disabledRaw *fmtItem
	for i := range npm.picker.items {
		if npm.picker.items[i].id == "raw" {
			disabledRaw = &npm.picker.items[i]
		}
	}
	if disabledRaw == nil || !disabledRaw.disabled {
		t.Fatal("Log picker should list RAW disabled when there is no live sink")
	}
	// Space on the disabled RAW must not select it, and warns why.
	npm.picker.cursor = 0 // RAW is first
	nd, _ := npm.Update(keySpace)
	if p := nd.(tuiModel).picker; p.items[0].on {
		t.Error("space toggled a disabled item")
	} else if p.hint == "" {
		t.Error("space on a disabled item should hint why")
	}
	// Same outputs as Save Buffer minus the disabled RAW: csv + 4 grids.
	if got := npm.picker.selected(); len(got) != 5 || contains(got, "raw") {
		t.Errorf("selected = %v, want csv + 4 grids (disabled raw excluded)", got)
	}

	// Live model: sink present → RAW offered, off by default.
	dir := t.TempDir()
	live := testModel()
	live.recSink = &stream.RecordSink{}
	lp, _ := live.Update(runes('l'))
	lpm := lp.(tuiModel)
	var rawItem *fmtItem
	for i := range lpm.picker.items {
		if lpm.picker.items[i].id == "raw" {
			rawItem = &lpm.picker.items[i]
		}
	}
	if rawItem == nil {
		t.Fatal("Log picker should offer RAW on a live source")
	}
	if rawItem.disabled {
		t.Error("RAW should be enabled on a live source")
	}
	if rawItem.on {
		t.Error("RAW should default off")
	}
	// Select RAW (cursor starts on it), keep CSV on, set dir+name, confirm.
	lpm.picker.items[0].on = true // raw is first
	lpm.picker.dir, lpm.picker.name = dir, "drive"
	next, _ := lpm.Update(keyEnter)
	mm := next.(tuiModel)
	if mm.picker != nil {
		t.Fatalf("confirm should close the picker (hint %q)", lpm.picker.hint)
	}
	if mm.recFile == nil || !mm.recSink.Active() || mm.csvLog == nil {
		t.Fatal("confirm should open both the raw sink and the CSV log")
	}

	// Forward streaming: a ParseOK frame writes a CSV row; a bad frame doesn't;
	// the raw sink takes whatever the provider writes.
	mm.recSink.Write([]byte{0xFE, 0x00, 0xFE})
	var cur tea.Model = mm
	cur, _ = cur.Update(snapshotMsg(recordableSnapshot()))
	bad := stream.Snapshot{
		FrameEvent: stream.FrameEvent{Frame: decoder.Frame{Data: []byte{0xDE, 0xAD}}, Index: 1},
		ParseOK:    false,
	}
	cur, _ = cur.Update(snapshotMsg(bad))
	cur, _ = cur.Update(snapshotMsg(recordableSnapshot()))
	mm2 := cur.(tuiModel)
	if mm2.csvLog.Rows != 2 {
		t.Errorf("csv rows = %d, want 2 (ParseOK only)", mm2.csvLog.Rows)
	}

	// Crash-proof: the aggregate tables are already complete on disk mid-log,
	// before any stop — a crash here would still leave a usable BLM table.
	if b, err := os.ReadFile(filepath.Join(dir, "drive_BLM.txt")); err != nil || !strings.Contains(string(b), "Wide Average BLM") {
		t.Errorf("drive_BLM.txt not written live mid-log: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drive_BLM.txt.tmp")); err == nil {
		t.Error("temp file left behind — atomic rename should remove it")
	}

	// Second `l` stops streams + finalises grids, recording all for the summary.
	next2, _ := mm2.Update(runes('l'))
	mm3 := next2.(tuiModel)
	if mm3.recFile != nil || mm3.recSink.Active() || mm3.csvLog != nil {
		t.Fatal("second l should stop the log")
	}
	if mm3.logActive() {
		t.Error("stop should clear pending grid logging")
	}
	if len(mm3.written) != 6 { // raw + csv + 4 grids
		t.Errorf("written records = %d, want 6 (raw + csv + 4 grids)", len(mm3.written))
	}
	if !strings.Contains(mm3.notice, "stopped log") {
		t.Errorf("notice = %q, want stopped log", mm3.notice)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "drive.raw")); err != nil || len(b) != 3 {
		t.Errorf("drive.raw = %d bytes (%v), want 3", len(b), err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "drive.csv"))
	if err != nil {
		t.Fatalf("read drive.csv: %v", err)
	}
	if lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n"); len(lines) != 3 {
		t.Errorf("csv has %d lines, want 3 (header + 2 rows)", len(lines))
	}
	for _, s := range []string{"BLM", "INT", "O2", "SPARK"} {
		if _, err := os.Stat(filepath.Join(dir, "drive_"+s+".txt")); err != nil {
			t.Errorf("missing %s grid file: %v", s, err)
		}
	}
}

// TestSaveBufferCSV: the retroactive property — with NO live CSV ever open, a
// Save Buffer CSV dumped from the frame ring is byte-identical to a live CSV
// written over the same frames.
func TestSaveBufferCSV(t *testing.T) {
	dir := t.TempDir()
	frames := []stream.Snapshot{recordableSnapshot(), recordableSnapshot(), recordableSnapshot()}

	// (a) Reference: a live frameCSV over the frames (the retired `d` path).
	ref := filepath.Join(dir, "ref.csv")
	c, err := newFrameCSV(ref, testModel().def)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range frames {
		c.Write(s.Elapsed.Seconds(), s.Frame.ByteOffset, s.PROMOK, s.Sensors)
	}
	c.Close()

	// (b) Retroactive: feed the frames into a model that never logs, then Save
	// Buffer just the CSV.
	var cur tea.Model = testModel()
	for _, s := range frames {
		cur, _ = cur.Update(snapshotMsg(s))
	}
	mm := cur.(tuiModel)
	if mm.csvLog != nil {
		t.Fatal("no live CSV should be open")
	}
	mm.picker = &outputPicker{op: opSaveBuffer, items: []fmtItem{{id: "csv", label: "Sensor CSV", on: true}}, name: filepath.Join(dir, "buf")}
	next, _ := mm.Update(keyEnter)
	if next.(tuiModel).picker != nil {
		t.Fatal("confirm should close the picker")
	}

	refBytes, _ := os.ReadFile(ref)
	bufBytes, err := os.ReadFile(filepath.Join(dir, "buf.csv"))
	if err != nil {
		t.Fatalf("read buf.csv: %v", err)
	}
	if string(bufBytes) != string(refBytes) {
		t.Errorf("Save Buffer CSV differs from live CSV:\n got %q\nwant %q", bufBytes, refBytes)
	}
}

// TestSaveBufferGridSubset: unchecking outputs saves only the selected grid
// (F18 single-grid save); a name collision keeps the picker open with nothing
// left on disk.
func TestSaveBufferGridSubset(t *testing.T) {
	dir := t.TempDir()
	next, _ := testModel().Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	// Only BLM selected.
	mm.picker = &outputPicker{op: opSaveBuffer, name: filepath.Join(dir, "one"), items: []fmtItem{
		{id: "csv", label: "Sensor CSV"},
		{id: "blm", label: "BLM grid", on: true},
		{id: "int", label: "INT grid"},
		{id: "o2", label: "O2 grid"},
		{id: "spark", label: "SPARK grid"},
	}}
	n2, _ := mm.Update(keyEnter)
	if n2.(tuiModel).picker != nil {
		t.Fatal("confirm should close")
	}
	if _, err := os.Stat(filepath.Join(dir, "one_BLM.txt")); err != nil {
		t.Errorf("BLM not written: %v", err)
	}
	for _, s := range []string{"INT", "O2", "SPARK"} {
		if _, err := os.Stat(filepath.Join(dir, "one_"+s+".txt")); err == nil {
			t.Errorf("%s written but not selected", s)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "one.csv")); err == nil {
		t.Error("csv written but not selected")
	}

	// Collision keeps the picker open, writes nothing new.
	mm2 := n2.(tuiModel)
	mm2.picker = &outputPicker{op: opSaveBuffer, name: filepath.Join(dir, "one"), items: []fmtItem{{id: "blm", label: "BLM grid", on: true}}}
	n3, _ := mm2.Update(keyEnter)
	if p := n3.(tuiModel).picker; p == nil || p.hint == "" {
		t.Error("collision should keep the picker open with a hint")
	}
}

// TestTUIQuitGuard (C.2): q is guarded when a Log is open or grids are unsaved —
// the first q arms + explains, a second quits; any other key disarms; ctrl+c is
// always immediate; a clean model quits on the first q.
func TestTUIQuitGuard(t *testing.T) {
	// Clean model → q quits immediately.
	if _, cmd := testModel().Update(runes('q')); cmd == nil {
		t.Error("clean q should quit immediately")
	}

	// Dirty model (a recordable frame accumulated grids).
	next, _ := testModel().Update(snapshotMsg(recordableSnapshot()))
	dirty := next.(tuiModel)
	if !dirty.unsaved() {
		t.Fatal("a recordable frame should leave grids unsaved")
	}
	n1, cmd1 := dirty.Update(runes('q'))
	m1 := n1.(tuiModel)
	if cmd1 != nil {
		t.Error("first q on a dirty model should not quit")
	}
	if !m1.quitArmed || !strings.Contains(m1.notice, "unsaved") {
		t.Errorf("first q should arm + notice, got armed=%v notice=%q", m1.quitArmed, m1.notice)
	}
	if _, cmd2 := m1.Update(runes('q')); cmd2 == nil {
		t.Error("second q should quit")
	}

	// A non-q key disarms; the following q re-arms (does not quit).
	armed := n1.(tuiModel)
	dis, _ := armed.Update(runes('2'))
	if dm := dis.(tuiModel); dm.quitArmed {
		t.Error("a non-q key should disarm the quit guard")
	}
	if _, cmd := dis.(tuiModel).Update(runes('q')); cmd != nil {
		t.Error("q after a disarm should re-arm, not quit")
	}

	// ctrl+c is immediate even when dirty.
	if _, cmd := dirty.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Error("ctrl+c should quit immediately even when dirty")
	}

	// A Log open with no dirty grids still guards.
	live := testModel()
	live.recSink = &stream.RecordSink{}
	live.csvLog, live.csvName = &frameCSV{}, "x" // pretend a CSV log is open
	if !live.logActive() {
		t.Fatal("csvLog set should make logActive true")
	}
	if _, cmd := live.Update(runes('q')); cmd != nil {
		t.Error("q with a Log open should be guarded, not quit")
	}
}

// TestTUIClearUndo (C.3): c clears with an undo hint; u restores the last clear
// (one slot) and re-dirties; u with nothing warns.
func TestTUIClearUndo(t *testing.T) {
	next, _ := testModel().Update(snapshotMsg(recordableSnapshot()))
	base := next.(tuiModel)
	base.active = viewBLM
	if base.grid.TotalSamples() == 0 {
		t.Fatal("BLM should have samples")
	}

	c1, _ := base.Update(runes('c'))
	m1 := c1.(tuiModel)
	if m1.grid.TotalSamples() != 0 {
		t.Error("c should clear the BLM grid")
	}
	if !m1.canUndo || !strings.Contains(m1.notice, "undo") {
		t.Errorf("clear should arm undo + hint, got canUndo=%v notice=%q", m1.canUndo, m1.notice)
	}
	u1, _ := m1.Update(runes('u'))
	m2 := u1.(tuiModel)
	if m2.grid.TotalSamples() == 0 {
		t.Error("u should restore the BLM grid")
	}
	if m2.canUndo || !m2.dirtyGrids {
		t.Errorf("undo consumes the slot + re-dirties, got canUndo=%v dirty=%v", m2.canUndo, m2.dirtyGrids)
	}
	// Nothing left to undo → self-expiring warning.
	u2, cmd := m2.Update(runes('u'))
	if cmd == nil || !strings.Contains(u2.(tuiModel).notice, "nothing to undo") {
		t.Errorf("u with nothing should warn, got notice=%q", u2.(tuiModel).notice)
	}

	// One slot: clearing INT then BLM leaves only BLM undoable.
	b := next.(tuiModel)
	b.active = viewINT
	ci, _ := b.Update(runes('c'))
	cim := ci.(tuiModel)
	cim.active = viewBLM
	cb, _ := cim.Update(runes('c'))
	uu, _ := cb.(tuiModel).Update(runes('u'))
	um := uu.(tuiModel)
	if um.grid.TotalSamples() == 0 {
		t.Error("u should restore the most-recent clear (BLM)")
	}
	if um.intGrid.TotalSamples() != 0 {
		t.Error("INT stays cleared — only one undo slot")
	}
}

// TestTUIDirtyTracking (C.1): grids go dirty on accumulation, a grid-inclusive
// Save Buffer clears it, and clearing all grids leaves nothing unsaved.
func TestTUIDirtyTracking(t *testing.T) {
	next, _ := testModel().Update(snapshotMsg(recordableSnapshot()))
	if !next.(tuiModel).unsaved() {
		t.Fatal("grids should be unsaved after accumulating")
	}

	saved := next.(tuiModel)
	saved.picker = &outputPicker{op: opSaveBuffer, name: filepath.Join(t.TempDir(), "s"),
		items: []fmtItem{{id: "blm", label: "BLM grid", on: true}}}
	n2, _ := saved.Update(keyEnter)
	if n2.(tuiModel).unsaved() {
		t.Error("a grid save should clear the unsaved state")
	}

	cleared := next.(tuiModel)
	for _, v := range []view{viewBLM, viewINT, viewO2, viewSpark} {
		cleared.active = v
		x, _ := cleared.Update(runes('c'))
		cleared = x.(tuiModel)
	}
	if cleared.unsaved() {
		t.Error("cleared grids hold no data → not unsaved")
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

// TestTUILegendModeAware: the live legend is the constant baseline in both
// modes; replay disables the live-only [l] log (struck through) and adds a
// separate playback-nav row; [c] is labelled by the active tab and dropped where
// it's a no-op; and [l] on replay warns.
func TestTUILegendModeAware(t *testing.T) {
	// Live: [l] log enabled; [c] labelled by the active grid.
	live := testModel()
	live.recSink = &stream.RecordSink{}
	live.active = viewBLM
	lg := live.keyLegend()
	if !strings.Contains(lg, dimStyle.Render("[l] log")) {
		t.Error("live legend should show [l] log enabled")
	}
	if !strings.Contains(lg, "[c] clear BLM") {
		t.Error("live legend should label clear by the active grid")
	}

	// Replay: the live legend still shows [l] log but disabled (struck); the
	// playback keys live on the separate replayNav row with the current speed.
	rp := testModel()
	rp.replay = &stream.ReplayProvider{Speed: 4}
	rp.active = viewSensors
	rg := rp.keyLegend()
	if !strings.Contains(rg, offStyle.Render("[l] log")) {
		t.Error("replay legend should show [l] log disabled (struck through)")
	}
	if !strings.Contains(rg, "[c] reset min/max") {
		t.Error("replay sensor-tab legend should label clear as reset min/max")
	}
	if nav := rp.replayNav(); !strings.Contains(nav, "[space] pause") || !strings.Contains(nav, "speed (4×)") {
		t.Errorf("replayNav %q missing pause/speed(4×)", nav)
	}
	// The playback row is only present in replay: live has no replayNav in its
	// footer (chrome is one row shorter).
	if live.chromeHeight() != 6 || rp.chromeHeight() != 7 {
		t.Errorf("chromeHeight live=%d replay=%d, want 6/7", live.chromeHeight(), rp.chromeHeight())
	}

	// [c] is dropped on tabs where it does nothing (flags/codes/raw).
	rp.active = viewFlags
	if strings.Contains(rp.keyLegend(), "[c]") {
		t.Error("flags-tab legend should have no [c]")
	}

	// [l] on replay warns and does not open the picker.
	rp2 := testModel()
	rp2.replay = &stream.ReplayProvider{Speed: 1}
	n, cmd := rp2.Update(runes('l'))
	if nm := n.(tuiModel); nm.picker != nil {
		t.Error("[l] on replay should not open the Log picker")
	} else if !strings.Contains(nm.notice, "live-only") {
		t.Errorf("[l] on replay notice = %q, want live-only", nm.notice)
	}
	if cmd == nil {
		t.Error("[l] on replay should arm a self-expiring warning")
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

	// Every tab renders with the title bar present.
	for v := view(0); v < viewCount; v++ {
		mm.active = v
		out := mm.View()
		if !strings.Contains(out, "GoALDL") {
			t.Errorf("tab %d missing the title bar", v)
		}
	}
}

// TestTUILoopLineHoldsLastGood: the loop badge (footer status line) reflects the
// TestTUIHeartbeatQuality: the heartbeat classifies recent stream quality — good
// (green) when the recent window is essentially all-parseable, warn (amber) on
// some recent loss, bad (red) on heavy loss — and recovers as bad frames age out
// of the window (the fix for "always yellow" from a rough start). The loop badge
// carries a filled/hollow state circle.
func TestTUIHeartbeatQuality(t *testing.T) {
	badFrame := stream.Snapshot{FrameEvent: stream.FrameEvent{Frame: decoder.Frame{Data: []byte{0xDE}}}, ParseOK: false}
	feed := func(m tea.Model, snap stream.Snapshot, n int) tea.Model {
		for i := 0; i < n; i++ {
			m, _ = m.Update(snapshotMsg(snap))
		}
		return m
	}

	// No frames yet → neutral, not warn/bad.
	if got := testModel().healthLevel(); got != healthNone {
		t.Errorf("pre-frame level = %v, want healthNone", got)
	}
	// A full window of good frames → good (green).
	allGood := feed(testModel(), recordableSnapshot(), healthWindowSize).(tuiModel)
	if got := allGood.healthLevel(); got != healthGood {
		t.Errorf("all-good level = %v, want healthGood", got)
	}
	// A rough start (several bad) then a full clean window → recovers to good;
	// this is the case that was stuck amber under the old cumulative ratio.
	rough := feed(testModel(), badFrame, 5)
	rough = feed(rough, recordableSnapshot(), healthWindowSize)
	if got := rough.(tuiModel).healthLevel(); got != healthGood {
		t.Errorf("recovered level = %v, want healthGood (bad frames aged out)", got)
	}
	// 4 bad in the last 30 (≈87% good) → amber; a heavy-loss window → red.
	amber := feed(feed(testModel(), badFrame, 4), recordableSnapshot(), 26)
	if got := amber.(tuiModel).healthLevel(); got != healthWarn {
		t.Errorf("4/30-bad level = %v, want healthWarn", got)
	}
	red := feed(testModel(), badFrame, healthWindowSize)
	if got := red.(tuiModel).healthLevel(); got != healthBadL {
		t.Errorf("all-bad level = %v, want healthBadL", got)
	}

	// Loop badge state circles: ● closed, ○ open.
	closed := feed(testModel(), recordableSnapshot(), 1).(tuiModel)
	if !strings.Contains(closed.styledLoopBadge(), "● ") {
		t.Errorf("closed-loop badge %q should lead with a filled ●", closed.styledLoopBadge())
	}
	ol := recordableSnapshot()
	ol.Frame.Data[14] = 0 // MWAF1=0 → open loop
	ol.FuelTrim = ecm.FuelTrimSample(ol.Frame.Data)
	on, _ := testModel().Update(snapshotMsg(ol))
	if !strings.Contains(on.(tuiModel).styledLoopBadge(), "○ ") {
		t.Errorf("open-loop badge %q should lead with a hollow ○", on.(tuiModel).styledLoopBadge())
	}
}

// last parseable frame and does not flicker on a following bad frame.
func TestTUILoopLineHoldsLastGood(t *testing.T) {
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot())) // closed loop
	mm := next.(tuiModel)
	if !strings.Contains(mm.styledLoopBadge(), "CLOSED LOOP") {
		t.Errorf("loop badge = %q, want CLOSED LOOP", mm.styledLoopBadge())
	}
	if mm.tabDot(viewBLM) != " ●" {
		t.Errorf("BLM tab dot = %q, want ● (closed loop, block-learn enabled)", mm.tabDot(viewBLM))
	}
	if mm.tabDot(viewSensors) != "" {
		t.Errorf("non-grid tab dot = %q, want empty", mm.tabDot(viewSensors))
	}

	bad := stream.Snapshot{
		FrameEvent: stream.FrameEvent{Frame: decoder.Frame{Data: []byte{0xDE, 0xAD}}, Index: 1},
		ParseOK:    false,
	}
	next2, _ := mm.Update(snapshotMsg(bad))
	mm2 := next2.(tuiModel)
	if !strings.Contains(mm2.styledLoopBadge(), "CLOSED LOOP") {
		t.Errorf("after bad frame loop badge = %q, want held CLOSED LOOP", mm2.styledLoopBadge())
	}
	// The dot also holds the last-good loop state (not flickered by the bad frame).
	if mm2.tabDot(viewBLM) != " ●" {
		t.Errorf("after bad frame BLM dot = %q, want held ●", mm2.tabDot(viewBLM))
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
// the arrow keys, and switching tabs re-homes the scroll.
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

	// ↓ scrolls down; the position indicator advances.
	down, _ := mm.Update(keyDown)
	dm := down.(tuiModel)
	if dm.scroll != 1 {
		t.Errorf("↓ should scroll down one line, got %d", dm.scroll)
	}
	// ↑ cannot scroll above the top.
	up, _ := mm.Update(keyUp)
	if s := up.(tuiModel).scroll; s != 0 {
		t.Errorf("↑ at the top should stay at 0, got %d", s)
	}
	// scroll is clamped to maxScroll — hammering ↓ never runs past the end.
	hammered := dm
	for i := 0; i < 500; i++ {
		h, _ := hammered.Update(keyDown)
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

// TestTUIFrameHeight: the frame is always exactly the terminal height (footer
// pinned to the last row, body padded or scrolled to fill) — so resizing can't
// leave a frozen copy of the old footer behind. The footer is two lines: the
// live status, then the key legend.
func TestTUIFrameHeight(t *testing.T) {
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)
	for _, h := range []int{12, 20, 30, 44} {
		sized, _ := mm.Update(tea.WindowSizeMsg{Width: 80, Height: h})
		fm := sized.(tuiModel)
		fm.active = viewSensors
		lines := strings.Split(fm.View(), "\n")
		if len(lines) != h {
			t.Errorf("height %d: frame has %d lines, want exactly %d", h, len(lines), h)
		}
		if last := lines[len(lines)-1]; !strings.Contains(last, "[s] save") {
			t.Errorf("height %d: last line should be the key legend, got %q", h, last)
		}
		// Title bar (brand + status) on line 0, blank line 1, tab bar line 2.
		if !strings.Contains(lines[0], "GoALDL") || !strings.Contains(lines[0], "Signal:") {
			t.Errorf("height %d: title bar should carry the brand + status, got %q", h, lines[0])
		}
		if !strings.Contains(lines[2], "Sensors") {
			t.Errorf("height %d: line 2 should be the tab bar, got %q", h, lines[2])
		}
	}

	// WindowSizeMsg re-clamps the scroll (a grown terminal can't leave scroll
	// pointing past the new end) and requests a clear to wipe stale rows.
	tall := mm
	tall.active, tall.scroll = viewRaw, 999
	sized, cmd := tall.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if s := sized.(tuiModel).scroll; s != sized.(tuiModel).maxScroll() {
		t.Errorf("resize should re-clamp scroll to maxScroll, got %d/%d", s, sized.(tuiModel).maxScroll())
	}
	if cmd == nil {
		t.Error("resize should return a command (ClearScreen) to wipe stale rows")
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
	if !strings.Contains(mm.signalDot(), "○") {
		t.Errorf("stale signal should be the hollow glyph, got %q", mm.signalDot())
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
	if !strings.Contains(mm.View(), "counter free-running — cell values are not knock") {
		t.Error("Spark tab should warn (in the legend) when the counter is free-running")
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
