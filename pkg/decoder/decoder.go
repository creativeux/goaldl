// Package decoder implements 160-baud ALDL decoding from a standard UART
// byte stream, using byte VALUES only — never host-side timing.
//
// Physical model: the ALDL line idles HIGH. Each 6250μs bit cell starts with
// a falling edge followed by a LOW pulse whose duration encodes the bit
// (short ≈365μs = logic 0, long ≈1875μs on the GM 1227747 = logic 1; widths
// vary by ECM family). The interface cable inverts the signal onto the PC
// UART's RX line, so the UART frames one character per ALDL bit: the falling
// edge is its start bit, and the number of consecutive LOW data bits (LSB
// first) measures the pulse width with the UART's own hardware clock.
//
// At 4800 baud (208μs/bit): logic 0 → 0xFE, logic 1 (1875μs = start + 8 data
// bits) → 0x00. At 2400 baud: logic 0 → 0xFF, logic 1 → ~0xF0/0xF8. Idle
// time between pulses produces no bytes at all, so the byte stream is NOT a
// uniform-rate waveform sample — each byte is one bit event.
//
// Character framing (per Tech Edge aldl160 spec): 9-bit characters = 1 mode
// bit + 8 data bits MSB first. Data characters have mode bit 0. The sync
// character is 0x1FF — nine consecutive 1 bits, the only place that pattern
// occurs — and separates 20-byte frames.
//
// Caveat: ECMs with very long logic-1 pulses (up to ~4.4ms) outlast one UART
// character at 4800 baud and may arrive as two 0x00 bytes (with a framing
// error). If that happens, runs of 1-bits double and frames abort on mode-bit
// errors — visible in Stats.FramesAborted. Record at 2400 baud instead, or
// extend ClassifyByte with run collapsing.
package decoder

import "math/bits"

// pulseThresholdMicros separates short (logic 0) from long (logic 1) pulses.
// Chosen midway between the widest observed short pulse (~500μs) and the
// narrowest observed long pulse (~1557μs, C3 spec) so it works across ECMs.
const pulseThresholdMicros = 1100

// Config describes how a raw capture was sampled.
type Config struct {
	BaudRate  int  // UART sampling rate the capture was recorded at (e.g. 4800)
	FrameSize int  // data bytes per frame (20 for GM 1227747)
	SyncBits  int  // consecutive 1-bits that constitute sync (9)
	Invert    bool // invert byte values first (non-inverting cable)
}

// DefaultConfig returns the configuration for the GM 1227747 recorded at 4800 baud.
func DefaultConfig() Config {
	return Config{BaudRate: 4800, FrameSize: 20, SyncBits: 9}
}

func (c Config) bitMicros() float64 { return 1e6 / float64(c.BaudRate) }

// ClassifyByte converts one received UART byte into one ALDL bit.
//
// The low pulse occupies the start bit plus the k lowest data bits, so a
// clean byte has the form 0xFF<<k and the pulse width is ≈(k+1) UART bit
// times. clean is false for any other bit pattern (line noise or a pulse
// edge landing exactly on a sample point); the bit value is still the best
// estimate from the trailing-zero count.
func (c Config) ClassifyByte(b byte) (bit byte, clean bool) {
	if c.Invert {
		b = ^b
	}
	k := bits.TrailingZeros8(b) // 8 when b == 0x00
	pulseMicros := float64(k+1) * c.bitMicros()
	if pulseMicros >= pulseThresholdMicros {
		bit = 1
	}
	clean = b == byte(0xFF<<k)
	return bit, clean
}

// Frame is one decoded ALDL frame.
type Frame struct {
	Data []byte
	// ByteOffset is the offset in the input byte stream at which the frame's
	// sync pattern ended (for diagnostics against the raw capture).
	ByteOffset int64
}

// Stats accumulates decoder diagnostics.
type Stats struct {
	BytesIn       int64 // total bytes fed
	NoisyBytes    int64 // bytes that were not a clean 0xFF<<k pattern
	SyncsFound    int64 // sync patterns (>= SyncBits consecutive 1s) seen
	FramesEmitted int64 // complete frames returned
	FramesAborted int64 // frames discarded due to a mode-bit error mid-frame
}

// Decoder is a streaming byte→frame state machine. Feed it raw UART bytes in
// order; it returns a Frame whenever one completes.
type Decoder struct {
	cfg   Config
	Stats Stats

	onesRun    int    // consecutive 1-bits seen while hunting for sync
	synced     bool   // collecting frame data
	char       uint16 // current 9-bit character accumulator
	charLen    int    // bits collected into char (0 = expecting mode bit)
	frame      []byte
	frameStart int64 // Stats.BytesIn when the current frame's sync ended
}

// New creates a streaming decoder.
func New(cfg Config) *Decoder {
	return &Decoder{cfg: cfg, frame: make([]byte, 0, cfg.FrameSize)}
}

// Feed processes one raw UART byte and returns a completed frame, or nil.
func (d *Decoder) Feed(b byte) *Frame {
	d.Stats.BytesIn++
	bit, clean := d.cfg.ClassifyByte(b)
	if !clean {
		d.Stats.NoisyBytes++
	}
	return d.feedBit(bit)
}

// Decode runs a whole capture through a fresh pass, returning all frames.
func (d *Decoder) Decode(data []byte) []Frame {
	var frames []Frame
	for _, b := range data {
		if f := d.Feed(b); f != nil {
			frames = append(frames, *f)
		}
	}
	return frames
}

func (d *Decoder) feedBit(bit byte) *Frame {
	if !d.synced {
		if bit == 1 {
			d.onesRun++
			return nil
		}
		// A 0-bit ends the run. If the run was long enough it was the sync
		// character, and this 0 is the mode bit of the frame's first byte.
		if d.onesRun >= d.cfg.SyncBits {
			d.Stats.SyncsFound++
			d.synced = true
			d.frame = d.frame[:0]
			d.frameStart = d.Stats.BytesIn
			d.char = 0
			d.charLen = 1 // mode bit consumed
		}
		d.onesRun = 0
		return nil
	}

	if d.charLen == 0 {
		// Expecting a mode bit. Data characters carry mode bit 0; a 1 here
		// mid-frame means we lost alignment (or hit an early sync).
		if bit == 1 {
			d.Stats.FramesAborted++
			d.synced = false
			d.onesRun = 1 // this bit may begin the next sync run
			return nil
		}
		d.charLen = 1
		return nil
	}

	// Data bits arrive MSB first.
	d.char = d.char<<1 | uint16(bit)
	d.charLen++
	if d.charLen < 9 {
		return nil
	}
	d.frame = append(d.frame, byte(d.char))
	d.char = 0
	d.charLen = 0

	if len(d.frame) < d.cfg.FrameSize {
		return nil
	}
	d.Stats.FramesEmitted++
	d.synced = false
	d.onesRun = 0
	out := &Frame{Data: append([]byte(nil), d.frame...), ByteOffset: d.frameStart}
	return out
}
