package stream

import (
	"context"
	"time"

	"goaldl/pkg/decoder"
)

// ReplayProvider streams frames decoded from a captured raw byte buffer,
// paced to wall-clock time so the table updates as if the data were live.
type ReplayProvider struct {
	Data   []byte
	Config decoder.Config
	// Speed scales playback: 1.0 = real time, 2.0 = twice as fast, 0 = no
	// pacing (emit as fast as possible; used by tests).
	Speed float64

	// now and sleep are injectable for deterministic tests; nil uses the real
	// clock. sleep must return early if ctx is cancelled.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

// NewReplayProvider builds a replay provider at real-time speed.
func NewReplayProvider(data []byte, cfg decoder.Config) *ReplayProvider {
	return &ReplayProvider{Data: data, Config: cfg, Speed: 1.0}
}

func (p *ReplayProvider) Name() string { return "replay" }

func (p *ReplayProvider) Run(ctx context.Context, emit func(FrameEvent)) error {
	now := p.now
	if now == nil {
		now = time.Now
	}
	sleep := p.sleep
	if sleep == nil {
		sleep = ctxSleep
	}

	frames := decoder.New(p.Config).Decode(p.Data)
	start := now()
	for i, f := range frames {
		// Each capture byte is one ALDL bit at 160 bps, so a frame's byte
		// offset maps directly to its position in the original recording.
		// This is what the consumer sees as Elapsed — the data timeline, not
		// how long playback has been running — so it is independent of Speed.
		dataElapsed := time.Duration(float64(f.ByteOffset) / 160.0 * float64(time.Second))
		if p.Speed > 0 {
			// Speed only compresses the wall-clock wait between frames.
			target := time.Duration(float64(dataElapsed) / p.Speed)
			if wait := target - now().Sub(start); wait > 0 {
				if err := sleep(ctx, wait); err != nil {
					return err
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		emit(FrameEvent{Frame: f, Index: i, Elapsed: dataElapsed})
	}
	return nil
}

// ctxSleep waits d, returning early with ctx.Err() if cancelled.
func ctxSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
