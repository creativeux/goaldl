package stream

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goaldl/pkg/aldl"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
)

func driveCapture(t *testing.T) []byte {
	t.Helper()
	// Fixtures live in pkg/decoder/testdata; reference them from here.
	raw, err := os.ReadFile(filepath.Join("..", "decoder", "testdata", "drive_4800.raw"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return raw
}

// TestReplayProviderEmitsAllFrames: with pacing off, the replay provider must
// emit exactly the frames the decoder finds, in order.
func TestReplayProviderEmitsAllFrames(t *testing.T) {
	data := driveCapture(t)
	cfg := decoder.DefaultConfig()
	wantFrames := decoder.New(cfg).Decode(data)

	p := &ReplayProvider{Data: data, Config: cfg, Speed: 0} // no pacing
	var got []FrameEvent
	if err := p.Run(context.Background(), func(ev FrameEvent) { got = append(got, ev) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(got) != len(wantFrames) {
		t.Fatalf("emitted %d frames, want %d", len(got), len(wantFrames))
	}
	for i, ev := range got {
		if ev.Index != i {
			t.Errorf("event %d has Index %d", i, ev.Index)
		}
		if int(ev.Frame.Data[1])<<8|int(ev.Frame.Data[2]) != 6291 {
			t.Errorf("frame %d PROM ID mismatch: % X", i, ev.Frame.Data[:3])
		}
	}
}

// TestReplayProviderPacing: a virtual clock proves frames are released at the
// times implied by their byte offsets, without real sleeping.
func TestReplayProviderPacing(t *testing.T) {
	data := driveCapture(t)
	cfg := decoder.DefaultConfig()

	var vclock time.Duration
	base := time.Unix(0, 0)
	p := &ReplayProvider{
		Data: data, Config: cfg, Speed: 1.0,
		now:   func() time.Time { return base.Add(vclock) },
		sleep: func(_ context.Context, d time.Duration) error { vclock += d; return nil },
	}

	frames := decoder.New(cfg).Decode(data)
	var lastElapsed time.Duration
	i := 0
	err := p.Run(context.Background(), func(ev FrameEvent) {
		// Elapsed must be monotonic and track the frame's original capture time.
		if ev.Elapsed < lastElapsed {
			t.Errorf("frame %d elapsed went backwards: %v < %v", ev.Index, ev.Elapsed, lastElapsed)
		}
		wantAt := time.Duration(float64(frames[ev.Index].ByteOffset) / 160.0 * float64(time.Second))
		if ev.Elapsed < wantAt-time.Millisecond {
			t.Errorf("frame %d released at %v, before its capture time %v", ev.Index, ev.Elapsed, wantAt)
		}
		lastElapsed = ev.Elapsed
		i++
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if i != len(frames) {
		t.Fatalf("emitted %d frames, want %d", i, len(frames))
	}
}

// TestReplayProviderCancel: a cancelled context stops the stream promptly.
func TestReplayProviderCancel(t *testing.T) {
	data := driveCapture(t)
	ctx, cancel := context.WithCancel(context.Background())
	p := &ReplayProvider{
		Data: data, Config: decoder.DefaultConfig(), Speed: 1.0,
		now:   func() time.Time { return time.Unix(0, 0) }, // clock never advances → always must sleep
		sleep: func(c context.Context, _ time.Duration) error { return c.Err() },
	}
	cancel()
	var count int
	err := p.Run(ctx, func(FrameEvent) { count += 1 })
	if err != context.Canceled {
		t.Errorf("Run returned %v, want context.Canceled", err)
	}
	if count != 0 {
		t.Errorf("emitted %d frames after cancel, want 0", count)
	}
}

// TestBuildRows checks the pure row builder against a known frame.
func TestBuildRows(t *testing.T) {
	// A real drive frame: PROM 24/147, coolant byte 0x53=83 → 158°F, RPM byte
	// 0x43=67 → 1675, battery byte 0x87=135 → 13.5V.
	frame := []byte{0x04, 0x18, 0x93, 0x75, 0x53, 0x00, 0x5B, 0x43, 0x36, 0x80, 0x69, 0x00, 0x00, 0x00, 0x00, 0x87, 0x80, 0x70, 0x7D, 0xC8}
	registry := ecm.NewRegistry()
	def, _ := registry.GetDefinition("1227747")
	data, err := parseHelper(registry, frame)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rows := BuildRows(frame, def, data)

	find := func(sensor string) Row {
		for _, r := range rows {
			if strings.Contains(strings.ToLower(r.Sensor), sensor) {
				return r
			}
		}
		t.Fatalf("no row for %q", sensor)
		return Row{}
	}

	rpm := find("engine speed")
	if !strings.Contains(rpm.Raw, "0x43") || !strings.Contains(rpm.Value, "1675") {
		t.Errorf("RPM row = %+v, want raw 0x43 / value 1675", rpm)
	}
	batt := find("battery")
	if !strings.Contains(batt.Value, "13.50") || !strings.Contains(batt.Value, "V") {
		t.Errorf("battery row = %+v, want 13.50 V", batt)
	}
	prom := find("prom")
	if !strings.Contains(prom.Raw, "0x18") || !strings.Contains(prom.Raw, "0x93") {
		t.Errorf("PROM row raw = %q, want both bytes", prom.Raw)
	}
}

func parseHelper(r *ecm.Registry, frame []byte) (map[string]float64, error) {
	d, err := r.ParseFrame(&aldl.Frame{Data: frame}, "1227747")
	if err != nil {
		return nil, err
	}
	return d.ParsedValues, nil
}
