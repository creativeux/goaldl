package stream

import (
	"bytes"
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

// TestReplayProviderEmitsAllFrames verifies the replay provider (the code under
// test) relays every frame from the decoder, in order and unmodified. The
// provider's contract is "decode the bytes, emit each frame", so the decoder's
// output is the oracle here (the decoder itself is independently validated in
// pkg/decoder against real captures). This catches the provider dropping,
// reordering, or corrupting frames — none of which the decoder can cause.
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
		if !bytes.Equal(ev.Frame.Data, wantFrames[i].Data) {
			t.Errorf("frame %d relayed wrong bytes:\n got % X\nwant % X", i, ev.Frame.Data, wantFrames[i].Data)
		}
		if int(ev.Frame.Data[1])<<8|int(ev.Frame.Data[2]) != 6291 {
			t.Errorf("frame %d PROM ID mismatch: % X", i, ev.Frame.Data[:3])
		}
	}
}

// TestReplayProviderPacing uses a virtual clock to prove two things across
// speeds: (1) each frame's Elapsed equals its data-timeline position (byte
// offset / 160), independent of speed; (2) the wall clock advances by that
// data time divided by speed — i.e. speed compresses playback, not the
// reported time.
func TestReplayProviderPacing(t *testing.T) {
	data := driveCapture(t)
	cfg := decoder.DefaultConfig()
	frames := decoder.New(cfg).Decode(data)
	dataTime := func(i int) time.Duration {
		return time.Duration(float64(frames[i].ByteOffset) / 160.0 * float64(time.Second))
	}

	for _, speed := range []float64{1.0, 5.0} {
		var vclock time.Duration
		base := time.Unix(0, 0)
		p := &ReplayProvider{
			Data: data, Config: cfg, Speed: speed,
			now:   func() time.Time { return base.Add(vclock) },
			sleep: func(_ context.Context, d time.Duration) error { vclock += d; return nil },
		}

		var lastElapsed time.Duration
		i := 0
		err := p.Run(context.Background(), func(ev FrameEvent) {
			if ev.Elapsed != dataTime(ev.Index) {
				t.Errorf("speed %v frame %d: Elapsed = %v, want data time %v",
					speed, ev.Index, ev.Elapsed, dataTime(ev.Index))
			}
			if ev.Elapsed < lastElapsed {
				t.Errorf("speed %v frame %d: elapsed went backwards", speed, ev.Index)
			}
			lastElapsed = ev.Elapsed
			i++
		})
		if err != nil {
			t.Fatalf("speed %v Run: %v", speed, err)
		}
		if i != len(frames) {
			t.Fatalf("speed %v: emitted %d frames, want %d", speed, i, len(frames))
		}
		// Wall clock (sum of sleeps) should be the last frame's data time / speed.
		wantWall := time.Duration(float64(dataTime(len(frames)-1)) / speed)
		if diff := vclock - wantWall; diff < -time.Millisecond || diff > time.Millisecond {
			t.Errorf("speed %v: wall clock advanced %v, want ~%v", speed, vclock, wantWall)
		}
	}
}

// TestReplayProviderPauseResume: pausing freezes playback (wall clock burns in
// pause slices, no frames emitted), resuming continues from the same data
// position with no catch-up rush — total wall time is the paced time plus
// exactly the paused time.
func TestReplayProviderPauseResume(t *testing.T) {
	data := driveCapture(t)
	cfg := decoder.DefaultConfig()
	frames := decoder.New(cfg).Decode(data)
	lastData := time.Duration(float64(frames[len(frames)-1].ByteOffset) / 160.0 * float64(time.Second))

	var vclock time.Duration
	base := time.Unix(0, 0)
	var p *ReplayProvider
	var pauseSlices int
	p = &ReplayProvider{
		Data: data, Config: cfg, Speed: 1.0,
		now: func() time.Time { return base.Add(vclock) },
		sleep: func(_ context.Context, d time.Duration) error {
			vclock += d
			if p.Paused() {
				pauseSlices++
				if pauseSlices == 5 {
					p.SetPaused(false)
				}
			}
			return nil
		},
	}

	var emitted int
	err := p.Run(context.Background(), func(ev FrameEvent) {
		emitted++
		if ev.Index == 3 {
			p.SetPaused(true)
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if emitted != len(frames) {
		t.Fatalf("emitted %d frames, want %d", emitted, len(frames))
	}
	if pauseSlices != 5 {
		t.Fatalf("pause slept %d slices, want 5", pauseSlices)
	}
	want := lastData + 5*controlSlice
	if diff := vclock - want; diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("wall clock = %v, want ~%v (paced + paused time)", vclock, want)
	}
}

// TestReplayProviderSpeedChange: SetSpeed applies from the current position
// only — frames before the change pace at the old rate, frames after at the
// new one, with no retroactive jump.
func TestReplayProviderSpeedChange(t *testing.T) {
	data := driveCapture(t)
	cfg := decoder.DefaultConfig()
	frames := decoder.New(cfg).Decode(data)
	dataTime := func(i int) time.Duration {
		return time.Duration(float64(frames[i].ByteOffset) / 160.0 * float64(time.Second))
	}
	const changeAt = 100

	var vclock time.Duration
	base := time.Unix(0, 0)
	p := &ReplayProvider{
		Data: data, Config: cfg, Speed: 1.0,
		now:   func() time.Time { return base.Add(vclock) },
		sleep: func(_ context.Context, d time.Duration) error { vclock += d; return nil },
	}

	var wallAtChange time.Duration
	err := p.Run(context.Background(), func(ev FrameEvent) {
		if ev.Index == changeAt {
			wallAtChange = vclock
			p.SetSpeed(2.0)
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := p.CurrentSpeed(); got != 2.0 {
		t.Errorf("CurrentSpeed = %v, want 2.0", got)
	}
	// Up to the change: real time. After: half time. No retroactive jump.
	if diff := wallAtChange - dataTime(changeAt); diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("wall at change = %v, want ~%v (old speed until the change)", wallAtChange, dataTime(changeAt))
	}
	want := dataTime(changeAt) + (dataTime(len(frames)-1)-dataTime(changeAt))/2
	if diff := vclock - want; diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("total wall = %v, want ~%v (2x only after the change)", vclock, want)
	}
}

// TestReplayProviderUnpacedControlsInert: with Speed 0 the provider never
// sleeps and runtime controls change nothing.
func TestReplayProviderUnpacedControlsInert(t *testing.T) {
	data := driveCapture(t)
	p := &ReplayProvider{
		Data: data, Config: decoder.DefaultConfig(), Speed: 0,
		sleep: func(context.Context, time.Duration) error {
			t.Error("unpaced replay slept")
			return nil
		},
	}
	var emitted int
	err := p.Run(context.Background(), func(ev FrameEvent) {
		if ev.Index == 0 {
			p.SetPaused(true) // must not stall an unpaced run
			p.SetSpeed(4.0)
		}
		emitted++
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if emitted == 0 {
		t.Fatal("no frames emitted")
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

	// Dual-unit Alt column: coolant 158°F → 70°C; MAP byte 0x5B=91 →
	// (91+28.06)/2.71 ≈ 43.93 kPa; TPS byte 0x36=54 → ~1.7% (default cal);
	// no Alt on RPM.
	ct := find("coolant")
	if !strings.Contains(ct.Alt, "70") || !strings.Contains(ct.Alt, "°C") {
		t.Errorf("coolant Alt = %q, want 70 °C", ct.Alt)
	}
	mapRow := find("map")
	if !strings.Contains(mapRow.Alt, "43.93") || !strings.Contains(mapRow.Alt, "kPa") {
		t.Errorf("MAP Alt = %q, want 43.93 kPa", mapRow.Alt)
	}
	tps := find("tps")
	if !strings.Contains(tps.Alt, "%") {
		t.Errorf("TPS Alt = %q, want a percent value", tps.Alt)
	}
	if rpm.Alt != "" {
		t.Errorf("RPM Alt = %q, want blank (no alternate)", rpm.Alt)
	}

	// Knock counter row exists (byte 17 = 0x70 = 112).
	knock := find("knock")
	if !strings.Contains(knock.Raw, "112") {
		t.Errorf("knock row = %+v, want raw 112", knock)
	}
}

func parseHelper(r *ecm.Registry, frame []byte) (map[string]float64, error) {
	d, err := r.ParseFrame(&aldl.Frame{Data: frame}, "1227747")
	if err != nil {
		return nil, err
	}
	return d.ParsedValues, nil
}
