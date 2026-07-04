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

// FrameEvent is one decoded frame delivered to a consumer, with the wall-clock
// time elapsed since the stream started and its sequence index.
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
