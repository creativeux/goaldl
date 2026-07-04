package stream

import (
	"context"
	"testing"
	"time"

	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
)

// recordableFrame is a closed-loop, block-learn-enabled GM 1227747 frame:
// PROM 24/147 (=6291), RPM 1600, MAP ~40 kPa, BLM 118, coolant byte 0x53.
func recordableFrame() []byte {
	f := make([]byte, 20)
	f[1], f[2] = 24, 147
	f[4] = 0x53 // coolant
	f[6], f[7] = 99, 64
	f[14], f[18] = 0x82, 118
	return f
}

func newTestSession(promID int) *Session {
	return NewSession(nil, ecm.NewRegistry(), "1227747", promID)
}

// TestSnapshotComposition checks the per-frame processing: parsed sensors,
// PROM check, parse-ok flag, and fuel-trim extraction.
func TestSnapshotComposition(t *testing.T) {
	s := newTestSession(6291)
	ev := FrameEvent{Frame: decoder.Frame{Data: recordableFrame()}, Index: 3, Elapsed: 2 * time.Second}

	snap := s.snapshot(ev)

	if !snap.PROMOK {
		t.Error("PROMOK should be true for PROM 6291")
	}
	if !snap.ParseOK {
		t.Error("ParseOK should be true for a valid 20-byte frame")
	}
	if snap.Index != 3 || snap.Elapsed != 2*time.Second {
		t.Errorf("FrameEvent not carried through: index=%d elapsed=%v", snap.Index, snap.Elapsed)
	}
	if got := snap.Sensors["engine_rpm"]; got != 1600 {
		t.Errorf("Sensors[engine_rpm] = %v, want 1600", got)
	}
	if !snap.FuelTrim.Recordable() || snap.FuelTrim.BLM != 118 {
		t.Errorf("FuelTrim = %+v, want recordable BLM 118", snap.FuelTrim)
	}
}

func TestSnapshotPROMMismatchAndDisable(t *testing.T) {
	f := recordableFrame()
	f[1], f[2] = 0, 0 // PROM now 0, not 6291

	if snap := newTestSession(6291).snapshot(FrameEvent{Frame: decoder.Frame{Data: f}}); snap.PROMOK {
		t.Error("PROMOK should be false when the frame PROM differs")
	}
	// promID 0 disables the check → always OK.
	if snap := newTestSession(0).snapshot(FrameEvent{Frame: decoder.Frame{Data: f}}); !snap.PROMOK {
		t.Error("PROMOK should be true when the check is disabled (promID 0)")
	}
}

// TestSnapshotShortFrame: a frame too short to parse yields ParseOK=false and
// empty Sensors, and PROMOK is false (FramePROM guards the short read).
func TestSnapshotShortFrame(t *testing.T) {
	snap := newTestSession(6291).snapshot(FrameEvent{Frame: decoder.Frame{Data: []byte{0x80, 0x18}}})
	if snap.ParseOK {
		t.Error("ParseOK should be false for a short frame")
	}
	if len(snap.Sensors) != 0 {
		t.Errorf("Sensors should be empty on parse failure, got %v", snap.Sensors)
	}
	if snap.PROMOK {
		t.Error("PROMOK should be false for a short frame")
	}
}

// TestSessionRun drives a real capture through a ReplayProvider and asserts the
// session emits one snapshot per decoded frame, in order, each fully processed.
func TestSessionRun(t *testing.T) {
	data := driveCapture(t)
	cfg := decoder.DefaultConfig()
	wantFrames := decoder.New(cfg).Decode(data)

	provider := &ReplayProvider{Data: data, Config: cfg, Speed: 0} // no pacing
	s := NewSession(provider, ecm.NewRegistry(), "1227747", 6291)

	var got []Snapshot
	if err := s.Run(context.Background(), func(snap Snapshot) { got = append(got, snap) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(got) != len(wantFrames) {
		t.Fatalf("emitted %d snapshots, want %d", len(got), len(wantFrames))
	}
	promMatches := 0
	for i, snap := range got {
		if snap.Index != i {
			t.Errorf("snapshot %d has Index %d", i, snap.Index)
		}
		if !snap.ParseOK {
			t.Errorf("snapshot %d ParseOK false", i)
		}
		if snap.PROMOK {
			promMatches++
		}
	}
	// The drive fixture is all PROM 6291.
	if promMatches != len(got) {
		t.Errorf("PROMOK on %d/%d snapshots, want all", promMatches, len(got))
	}
	if s.Name() != "replay" {
		t.Errorf("Name() = %q, want replay", s.Name())
	}
}

// TestSessionRunCancel: a cancelled context stops the session promptly.
func TestSessionRunCancel(t *testing.T) {
	data := driveCapture(t)
	ctx, cancel := context.WithCancel(context.Background())
	// Real-time pacing with a clock that never advances → the provider must
	// sleep before the first frame; cancelling makes that sleep return early.
	provider := &ReplayProvider{
		Data: data, Config: decoder.DefaultConfig(), Speed: 1.0,
		now:   func() time.Time { return time.Unix(0, 0) },
		sleep: func(c context.Context, _ time.Duration) error { return c.Err() },
	}
	s := NewSession(provider, ecm.NewRegistry(), "1227747", 6291)

	cancel()
	var count int
	if err := s.Run(ctx, func(Snapshot) { count++ }); err != context.Canceled {
		t.Errorf("Run returned %v, want context.Canceled", err)
	}
	if count != 0 {
		t.Errorf("emitted %d snapshots after cancel, want 0", count)
	}
}
