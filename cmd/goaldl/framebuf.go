package main

// frameBuf is the always-on, bounded ring of decoded frames the dashboard
// retains so a Save Buffer can export the session so far — the "I saw the
// anomaly and wasn't logging" case. It is ephemeral scratch by design: oldest
// frames drop as new ones arrive, so memory is bounded regardless of session
// length, and it is NOT treated as unsaved state (see the quit guard).

// frameBufCap is the ring capacity in frames — roughly one hour at the ~1.2s
// ALDL cadence. At ~200 B per bufFrame (20 frame bytes + ~20 float64 values)
// the full ring is under 1 MB.
const frameBufCap = 3600

// bufFrame is the compact projection the ring retains: the fields a CSV export
// needs, plus the aligned frame bytes so a future export can re-parse under a
// different ECM layout. Values are stored in def.Parameters order (no per-frame
// map) so memory is deterministic and the live Sensors map isn't kept alive.
type bufFrame struct {
	data       []byte    // 20 aligned frame bytes (copied at push)
	elapsedSec float64   // FrameEvent.Elapsed in seconds
	byteOffset int64     // frame position in the byte stream
	parseOK    bool      // frame decoded cleanly (drives ParseOK-only CSV rows)
	promOK     bool      // PROM ID matched
	vals       []float64 // parsed values, def.Parameters order (nil when !parseOK)
}

// frameBuf is a fixed-capacity ring. push overwrites the oldest entry once full;
// frames returns the retained window oldest-first; fillPct is how full it is.
type frameBuf struct {
	ring  []bufFrame
	head  int // next write index
	n     int // live count, capped at cap()
	total int // frames ever pushed (high-water)
}

func newFrameBuf() *frameBuf { return &frameBuf{ring: make([]bufFrame, frameBufCap)} }

func (b *frameBuf) cap() int { return len(b.ring) }

// push appends one frame, overwriting the oldest when the ring is full. O(1).
func (b *frameBuf) push(f bufFrame) {
	b.ring[b.head] = f
	b.head = (b.head + 1) % b.cap()
	if b.n < b.cap() {
		b.n++
	}
	b.total++
}

// frames returns the retained frames oldest-first — the order a CSV export
// writes them. The slice is freshly allocated (safe for the caller to hold).
func (b *frameBuf) frames() []bufFrame {
	out := make([]bufFrame, 0, b.n)
	// The oldest live entry sits head-n back from head (mod cap).
	start := (b.head - b.n + b.cap()) % b.cap()
	for i := 0; i < b.n; i++ {
		out = append(out, b.ring[(start+i)%b.cap()])
	}
	return out
}

// fillPct is the ring's fullness (0–100), saturating at 100 once it wraps.
func (b *frameBuf) fillPct() int {
	if b.cap() == 0 {
		return 0
	}
	return b.n * 100 / b.cap()
}
