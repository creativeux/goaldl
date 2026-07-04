package main

import (
	"errors"
	"strings"
	"testing"

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
		{runes('3'), viewFlags},
		{runes('4'), viewCodes},
		{runes('5'), viewRaw},
		{runes('1'), viewSensors},
		{tea.KeyMsg{Type: tea.KeyTab}, viewBLM},        // cycle forward
		{tea.KeyMsg{Type: tea.KeyTab}, viewFlags},      //
		{tea.KeyMsg{Type: tea.KeyTab}, viewCodes},      //
		{tea.KeyMsg{Type: tea.KeyTab}, viewRaw},        //
		{tea.KeyMsg{Type: tea.KeyTab}, viewSensors},    // wraps
		{tea.KeyMsg{Type: tea.KeyShiftTab}, viewRaw},   // cycle back, wraps
		{tea.KeyMsg{Type: tea.KeyShiftTab}, viewCodes}, //
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
	for _, want := range []string{"Sensors", "BLM grid", "Flags", "Codes", "Raw", "test", "PROM ✓", "1 ok / 0 bad"} {
		if !strings.Contains(v, want) {
			t.Errorf("sensors view missing %q", want)
		}
	}
	if !strings.Contains(v, "Engine speed") {
		t.Error("sensors view should render the sensor table")
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
