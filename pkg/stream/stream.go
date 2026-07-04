// Package stream feeds ALDL frames from a source (a live ECM or a replayed
// capture) into a presenter, so the same table can be driven by real hardware
// or by recorded bytes. A Provider is any source of decoded frames; the
// presenter (see table.go) is agnostic to which one it's fed.
package stream

import (
	"context"
	"time"

	"goaldl/pkg/decoder"
)

// FrameEvent is one decoded frame delivered to a consumer, with its position
// in the data timeline and its sequence index.
//
// Elapsed is the frame's time within the recording (for replay) or since the
// stream started (for live) — NOT how long playback has been running. For a
// replay at 5x speed, a frame captured at t=50s reports Elapsed=50s even
// though only 10s of wall-clock have passed. This keeps the displayed time and
// the exported CSV aligned with the source data regardless of playback speed.
type FrameEvent struct {
	Frame   decoder.Frame
	Index   int
	Elapsed time.Duration
}

// Provider is a source of decoded ALDL frames. Run streams frames to emit
// until the source is exhausted, ctx is cancelled, or an error occurs.
// Implementations must return ctx.Err() (or nil) on cancellation, never block
// past it.
type Provider interface {
	Name() string
	Run(ctx context.Context, emit func(FrameEvent)) error
}
