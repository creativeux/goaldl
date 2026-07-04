package stream

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"goaldl/pkg/decoder"
)

// TestBLMViewAccumulates: driving the view with the real drive capture must
// record the same number of closed-loop samples the blm command reports (469),
// proving the live view and batch command share one extraction/gating path.
func TestBLMViewAccumulates(t *testing.T) {
	data := driveCapture(t)
	view := NewBLMView(&bytes.Buffer{}, false, "test") // non-TTY: accumulate only
	p := &ReplayProvider{Data: data, Config: decoder.DefaultConfig(), Speed: 0}
	if err := p.Run(context.Background(), view.Render); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := view.Grid.TotalSamples(); got != 469 {
		t.Errorf("recorded %d samples, want 469", got)
	}
}

// TestBLMViewLiveRender: on a TTY, a recordable frame must draw the grid,
// highlight the active cell (reverse video), and show the closed-loop status.
func TestBLMViewLiveRender(t *testing.T) {
	var buf bytes.Buffer
	view := NewBLMView(&buf, true, "test")

	// RPM 64*25=1600, MAP byte 99 (~40 kPa), BLM 118, MWAF1=0x82 (closed+enable).
	f := make([]byte, 20)
	f[6], f[7], f[14], f[18] = 99, 64, 0x82, 118
	view.Render(FrameEvent{Frame: decoder.Frame{Data: f}})

	out := buf.String()
	if !strings.Contains(out, "CLOSED LOOP") {
		t.Errorf("output missing closed-loop status:\n%s", out)
	}
	if !strings.Contains(out, "\033[7m") {
		t.Error("output missing reverse-video highlight for the active cell")
	}
	if !strings.Contains(out, "RPM\\MAP") {
		t.Error("output missing grid header")
	}
	if view.Grid.TotalSamples() != 1 {
		t.Errorf("recorded %d, want 1", view.Grid.TotalSamples())
	}
}

// TestBLMViewOpenLoopNotRecorded: an open-loop frame updates the status line
// but records nothing.
func TestBLMViewOpenLoopNotRecorded(t *testing.T) {
	var buf bytes.Buffer
	view := NewBLMView(&buf, true, "test")
	f := make([]byte, 20)
	f[6], f[7], f[14], f[18] = 99, 64, 0x00, 118 // MWAF1=0 → open loop
	view.Render(FrameEvent{Frame: decoder.Frame{Data: f}})

	if view.Grid.TotalSamples() != 0 {
		t.Errorf("recorded %d, want 0 (open loop)", view.Grid.TotalSamples())
	}
	if !strings.Contains(buf.String(), "OPEN LOOP") {
		t.Error("output missing open-loop status")
	}
}
