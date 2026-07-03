package decoder

import (
	"bytes"
	"testing"
)

// Ground-truth frames taken verbatim from data/20250601_111156_LOG.txt
// (WinALDL log of the real GM 1227747 at PROM ID 24/147). Column order:
// MW2, PROMIDA, PROMIDB, IAC, CT, MPH, MAP, RPM, TPS, INT, O2,
// MALFFLG1-3, MWAF1, VOLT, MCU2IO, KNOCK_CNT, BLM, O2_CNT.
var groundTruthFrames = [][]byte{
	{128, 24, 147, 145, 190, 0, 183, 0, 28, 128, 44, 0, 0, 0, 0, 119, 128, 0, 128, 12},     // t=4.5s (key on, engine off)
	{128, 24, 147, 145, 190, 0, 81, 50, 28, 128, 144, 0, 0, 0, 64, 115, 128, 168, 125, 15}, // t=9.3s (running, 1250 RPM)
	{132, 24, 147, 145, 190, 0, 65, 49, 28, 128, 238, 0, 0, 0, 64, 128, 128, 244, 125, 15}, // t=10.4s
}

func TestClassifyByte(t *testing.T) {
	tests := []struct {
		baud      int
		invert    bool
		b         byte
		wantBit   byte
		wantClean bool
	}{
		// 4800 baud: 208μs/UART bit. Short pulse (~365μs) = 0xFE, long (~1875μs) = 0x00.
		{4800, false, 0xFE, 0, true},  // nominal logic 0
		{4800, false, 0xFC, 0, true},  // wider logic 0 (~625μs)
		{4800, false, 0xF0, 0, true},  // 1040μs — still below threshold
		{4800, false, 0xE0, 1, true},  // 1250μs — above threshold
		{4800, false, 0x00, 1, true},  // nominal logic 1
		{4800, false, 0x80, 1, true},  // shorter long pulse
		{4800, false, 0x55, 0, false}, // line noise: classified but flagged
		// 2400 baud: 417μs/UART bit. Short pulse fits inside the start bit = 0xFF.
		{2400, false, 0xFF, 0, true},
		{2400, false, 0xF8, 1, true}, // ~1667μs
		{2400, false, 0xF0, 1, true}, // ~2083μs
		// Inverted (non-inverting cable).
		{4800, true, 0x01, 0, true}, // ^0x01 = 0xFE
		{4800, true, 0xFF, 1, true}, // ^0xFF = 0x00
	}
	for _, tt := range tests {
		cfg := Config{BaudRate: tt.baud, FrameSize: 20, SyncBits: 9, Invert: tt.invert}
		bit, clean := cfg.ClassifyByte(tt.b)
		if bit != tt.wantBit || clean != tt.wantClean {
			t.Errorf("ClassifyByte(0x%02X) @%d invert=%v = (%d,%v), want (%d,%v)",
				tt.b, tt.baud, tt.invert, bit, clean, tt.wantBit, tt.wantClean)
		}
	}
}

func roundTrip(t *testing.T, cfg Config, enc *Encoder) {
	t.Helper()
	stream := enc.EncodeStream(groundTruthFrames)
	d := New(cfg)
	frames := d.Decode(stream)

	if len(frames) != len(groundTruthFrames) {
		t.Fatalf("decoded %d frames, want %d (stats: %+v)", len(frames), len(groundTruthFrames), d.Stats)
	}
	for i, f := range frames {
		if !bytes.Equal(f.Data, groundTruthFrames[i]) {
			t.Errorf("frame %d mismatch:\n got %v\nwant %v", i, f.Data, groundTruthFrames[i])
		}
	}
	if d.Stats.FramesAborted != 0 {
		t.Errorf("unexpected aborted frames: %+v", d.Stats)
	}
}

func TestRoundTrip4800(t *testing.T) {
	cfg := DefaultConfig()
	roundTrip(t, cfg, NewEncoder(cfg))
}

func TestRoundTrip2400(t *testing.T) {
	cfg := Config{BaudRate: 2400, FrameSize: 20, SyncBits: 9}
	roundTrip(t, cfg, NewEncoder(cfg))
}

func TestRoundTripInverted(t *testing.T) {
	cfg := Config{BaudRate: 4800, FrameSize: 20, SyncBits: 9, Invert: true}
	roundTrip(t, cfg, NewEncoder(cfg))
}

// Pulse widths vary between ECM families (and jitter on one ECM); the
// classifier must tolerate the full plausible range on either side of the
// 1100μs threshold.
func TestPulseWidthVariation(t *testing.T) {
	cfg := DefaultConfig()
	for _, short := range []float64{250, 365, 500, 900} {
		for _, long := range []float64{1300, 1557, 1875} {
			enc := NewEncoder(cfg)
			enc.ShortPulseMicros = short
			enc.LongPulseMicros = long
			t.Run("", func(t *testing.T) { roundTrip(t, cfg, enc) })
		}
	}
}

// A capture never starts cleanly at a frame boundary: prepend the tail half
// of a frame plus idle noise and make sure sync is still acquired.
func TestLeadingGarbage(t *testing.T) {
	cfg := DefaultConfig()
	enc := NewEncoder(cfg)
	partial := enc.EncodeFrame(groundTruthFrames[0])
	garbage := append([]byte{0x55, 0xFE, 0xFE}, partial[len(partial)/2:]...)
	stream := append(garbage, enc.EncodeStream(groundTruthFrames)...)

	d := New(cfg)
	frames := d.Decode(stream)
	if len(frames) != len(groundTruthFrames) {
		t.Fatalf("decoded %d frames, want %d (stats: %+v)", len(frames), len(groundTruthFrames), d.Stats)
	}
	for i, f := range frames {
		if !bytes.Equal(f.Data, groundTruthFrames[i]) {
			t.Errorf("frame %d mismatch: got %v", i, f.Data)
		}
	}
}

// Corrupting a mode-bit position must abort that frame and resync cleanly on
// the next one, not poison the rest of the capture.
func TestNoiseRecovery(t *testing.T) {
	cfg := DefaultConfig()
	enc := NewEncoder(cfg)
	stream := enc.EncodeStream(groundTruthFrames)
	stream[cfg.SyncBits] = 0x00 // frame 0's first mode bit: force it to read as 1

	d := New(cfg)
	frames := d.Decode(stream)
	if d.Stats.FramesAborted != 1 {
		t.Errorf("FramesAborted = %d, want 1 (stats: %+v)", d.Stats.FramesAborted, d.Stats)
	}
	if len(frames) != len(groundTruthFrames)-1 {
		t.Fatalf("decoded %d frames, want %d", len(frames), len(groundTruthFrames)-1)
	}
	for i, f := range frames {
		if !bytes.Equal(f.Data, groundTruthFrames[i+1]) {
			t.Errorf("frame %d mismatch: got %v", i, f.Data)
		}
	}
}
