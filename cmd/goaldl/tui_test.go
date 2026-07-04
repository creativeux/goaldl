package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/stream"
)

func testModel() tuiModel {
	return tuiModel{
		registry:   ecm.NewRegistry(),
		ecmPart:    "1227747",
		promID:     6291,
		minSamples: blm.DefaultMinSamples,
		source:     "test",
		cancel:     func() {},
		grid:       blm.NewDefault(),
	}
}

func runes(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// recordableFrame is a closed-loop, block-learn-enabled frame (MWAF1=0x82),
// PROM 24/147, RPM 1600, MAP ~40 kPa, BLM 118.
func recordableFrame() stream.FrameEvent {
	f := make([]byte, 20)
	f[1], f[2], f[6], f[7], f[14], f[18] = 24, 147, 99, 64, 0x82, 118
	return stream.FrameEvent{Frame: decoder.Frame{Data: f}, Index: 0}
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
		{runes('3'), viewRaw},
		{runes('1'), viewSensors},
		{tea.KeyMsg{Type: tea.KeyTab}, viewBLM},      // cycle forward
		{tea.KeyMsg{Type: tea.KeyTab}, viewRaw},      //
		{tea.KeyMsg{Type: tea.KeyTab}, viewSensors},  // wraps
		{tea.KeyMsg{Type: tea.KeyShiftTab}, viewRaw}, // cycle back, wraps
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
	next, cmd := m.Update(frameMsg(recordableFrame()))
	mm := next.(tuiModel)

	if !mm.hasFrame || mm.frameCount != 1 {
		t.Errorf("hasFrame=%v frameCount=%d, want true/1", mm.hasFrame, mm.frameCount)
	}
	if mm.grid.TotalSamples() != 1 {
		t.Errorf("grid recorded %d, want 1 (recordable frame)", mm.grid.TotalSamples())
	}
	if cmd == nil {
		t.Error("frameMsg should re-issue waitForFrame command")
	}

	// An open-loop frame (MWAF1=0) advances the frame count but records nothing.
	f := recordableFrame()
	f.Frame.Data[14] = 0
	next2, _ := mm.Update(frameMsg(f))
	mm2 := next2.(tuiModel)
	if mm2.frameCount != 2 {
		t.Errorf("frameCount = %d, want 2", mm2.frameCount)
	}
	if mm2.grid.TotalSamples() != 1 {
		t.Errorf("grid recorded %d after open-loop frame, want still 1", mm2.grid.TotalSamples())
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

	next, _ := m.Update(frameMsg(recordableFrame()))
	mm := next.(tuiModel)

	// Header always shows the tab bar and source.
	v := mm.View()
	for _, want := range []string{"Sensors", "BLM grid", "Raw", "test", "PROM ✓"} {
		if !strings.Contains(v, want) {
			t.Errorf("sensors view missing %q", want)
		}
	}
	if !strings.Contains(v, "Engine speed") {
		t.Error("sensors view should render the sensor table")
	}

	mm.active = viewBLM
	if !strings.Contains(mm.View(), "RPM\\MAP") {
		t.Error("BLM view should render the grid")
	}

	mm.active = viewRaw
	if !strings.Contains(mm.View(), "bytes:") {
		t.Error("raw view should show the frame bytes")
	}
}
