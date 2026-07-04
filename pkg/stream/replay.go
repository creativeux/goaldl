package stream

import (
	"context"
	"sync"
	"time"

	"goaldl/pkg/decoder"
)

// ReplayProvider streams frames decoded from a captured raw byte buffer,
// paced to wall-clock time so the table updates as if the data were live.
// Playback can be paused and re-paced at runtime (SetPaused/SetSpeed) — the
// TUI's space and +/- keys — unless Speed is 0 (unpaced), which renders the
// runtime controls inert.
type ReplayProvider struct {
	Data   []byte
	Config decoder.Config
	// Speed scales playback: 1.0 = real time, 2.0 = twice as fast, 0 = no
	// pacing (emit as fast as possible; used by tests). This is the initial
	// rate; see SetSpeed for runtime changes.
	Speed float64

	// now and sleep are injectable for deterministic tests; nil uses the real
	// clock. sleep must return early if ctx is cancelled.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error

	mu     sync.Mutex
	paused bool
	speed  float64 // runtime override; 0 = unset, use Speed
}

// NewReplayProvider builds a replay provider at real-time speed.
func NewReplayProvider(data []byte, cfg decoder.Config) *ReplayProvider {
	return &ReplayProvider{Data: data, Config: cfg, Speed: 1.0}
}

func (p *ReplayProvider) Name() string { return "replay" }

// SetPaused pauses (true) or resumes (false) playback. While paused the data
// position is frozen — resuming continues from where playback stopped, with
// no catch-up rush. Inert when Speed is 0.
func (p *ReplayProvider) SetPaused(v bool) {
	p.mu.Lock()
	p.paused = v
	p.mu.Unlock()
}

// Paused reports whether playback is currently paused.
func (p *ReplayProvider) Paused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

// SetSpeed changes the playback rate from the current position onward — the
// change is never retroactive (no jump to where the new rate "would have"
// put playback). Non-positive rates are ignored. Inert when Speed is 0.
func (p *ReplayProvider) SetSpeed(v float64) {
	if v <= 0 {
		return
	}
	p.mu.Lock()
	p.speed = v
	p.mu.Unlock()
}

// CurrentSpeed returns the effective playback rate: the last SetSpeed value,
// or the initial Speed before any runtime change (0 = unpaced).
func (p *ReplayProvider) CurrentSpeed() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.speed > 0 {
		return p.speed
	}
	return p.Speed
}

func (p *ReplayProvider) controls() (paused bool, speed float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	speed = p.speed
	if speed <= 0 {
		speed = p.Speed
	}
	return p.paused, speed
}

// controlSlice bounds each wait so a pause/speed toggle takes effect within
// ~100ms even in the middle of a long inter-frame gap.
const controlSlice = 100 * time.Millisecond

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
	if p.Speed <= 0 {
		// Unpaced: emit as fast as possible; runtime controls are inert.
		for i, f := range frames {
			if err := ctx.Err(); err != nil {
				return err
			}
			dataElapsed := time.Duration(float64(f.ByteOffset) / 160.0 * float64(time.Second))
			emit(FrameEvent{Frame: f, Index: i, Elapsed: dataElapsed})
		}
		return nil
	}

	// Pacing anchors: playback is at data position anchorData as of wall time
	// anchorWall, advancing at lastSpeed while running. Re-anchoring on every
	// control change is what makes a speed change apply only from the current
	// position, and a pause freeze the position rather than queue a catch-up.
	anchorWall := now()
	anchorData := time.Duration(0)
	lastPaused, lastSpeed := p.controls()
	for i, f := range frames {
		// Each capture byte is one ALDL bit at 160 bps, so a frame's byte
		// offset maps directly to its position in the original recording.
		// This is what the consumer sees as Elapsed — the data timeline, not
		// how long playback has been running — so it is independent of Speed.
		dataElapsed := time.Duration(float64(f.ByteOffset) / 160.0 * float64(time.Second))
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			paused, speed := p.controls()
			if paused != lastPaused || speed != lastSpeed {
				t := now()
				if !lastPaused {
					anchorData += time.Duration(float64(t.Sub(anchorWall)) * lastSpeed)
				}
				anchorWall = t
				lastPaused, lastSpeed = paused, speed
			}
			if paused {
				anchorWall = now() // hold the data position while paused
				if err := sleep(ctx, controlSlice); err != nil {
					return err
				}
				continue
			}
			wait := time.Duration(float64(dataElapsed-anchorData)/speed) - now().Sub(anchorWall)
			if wait <= 0 {
				break
			}
			if wait > controlSlice {
				wait = controlSlice
			}
			if err := sleep(ctx, wait); err != nil {
				return err
			}
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
