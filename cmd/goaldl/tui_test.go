package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
		{runes('5'), viewFlags},
		{runes('6'), viewCodes},
		{runes('7'), viewRaw},
		{runes('1'), viewSensors},
		{tea.KeyMsg{Type: tea.KeyTab}, viewBLM},      // cycle forward
		{tea.KeyMsg{Type: tea.KeyTab}, viewINT},      //
		{tea.KeyMsg{Type: tea.KeyTab}, viewO2},       //
		{tea.KeyMsg{Type: tea.KeyShiftTab}, viewINT}, // cycle back
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
	for _, want := range []string{"Sensors", "BLM", "INT", "O2", "Flags", "Codes", "Raw", "test", "PROM ✓", "1 ok / 0 bad"} {
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

// TestSaveGrids: writes three files; BLM/INT carry a correction table, O2 does
// not (it is a voltage).
func TestSaveGrids(t *testing.T) {
	m := testModel()
	next, _ := m.Update(snapshotMsg(recordableSnapshot()))
	mm := next.(tuiModel)

	dir := t.TempDir()
	ts := time.Date(2026, 7, 4, 14, 30, 22, 0, time.UTC)
	base, err := saveGrids(dir, ts, mm.grid, mm.intGrid, mm.o2Grid, mm.minSamples)
	if err != nil {
		t.Fatalf("saveGrids: %v", err)
	}
	if base != "goaldl_20260704_143022" {
		t.Errorf("base = %q, want goaldl_20260704_143022", base)
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
}

// TestTUIDriveFixtureEndToEnd drives the full real drive capture through a
// Session into the dashboard model, exercising the accumulation path over every
// frame. The BLM count must match the `blm` command's 469 (shared gating),
// INT (closed-loop only) must exceed it, O2 (ungated) must exceed INT, and all
// seven tabs must render without panic.
func TestTUIDriveFixtureEndToEnd(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "pkg", "decoder", "testdata", "drive_4800.raw"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	registry := ecm.NewRegistry()
	provider := &stream.ReplayProvider{Data: raw, Config: decoder.DefaultConfig(), Speed: 0}
	session := stream.NewSession(provider, registry, "1227747", 6291)

	var cur tea.Model = testModel()
	if err := session.Run(t.Context(), func(s stream.Snapshot) {
		cur, _ = cur.Update(snapshotMsg(s))
	}); err != nil {
		t.Fatalf("session run: %v", err)
	}
	mm := cur.(tuiModel)

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
