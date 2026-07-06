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

	// decodeOnce guards the one-time decode shared by Run and Duration, so the
	// capture is never decoded twice and the two can never disagree on the frame
	// list or total length.
	decodeOnce sync.Once
	frames     []decoder.Frame
	total      time.Duration // data-timeline length (last frame's Elapsed)

	mu     sync.Mutex
	paused bool
	speed  float64        // runtime override; 0 = unset, use Speed
	seekTo *time.Duration // pending seek target, applied at the next frame boundary
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

// frameElapsed maps a frame's byte offset to its data-timeline position — each
// capture byte is one ALDL bit at 160 bps, so the offset is the position in the
// original recording. This is what the consumer sees as Elapsed.
func frameElapsed(f decoder.Frame) time.Duration {
	return time.Duration(float64(f.ByteOffset) / 160.0 * float64(time.Second))
}

// ensureDecoded decodes the capture once (shared by Run and Duration) and caches
// the frames plus the total length. Safe to call from either goroutine.
func (p *ReplayProvider) ensureDecoded() {
	p.decodeOnce.Do(func() {
		p.frames = decoder.New(p.Config).Decode(p.Data)
		if n := len(p.frames); n > 0 {
			p.total = frameElapsed(p.frames[n-1])
		}
	})
}

// Duration returns the total data-timeline length of the capture (the last
// frame's Elapsed), or 0 for an empty capture. The first call decodes the
// capture once (O(n), cached and shared with Run via decodeOnce, so it is never
// paid twice); subsequent calls are O(1). Callable before Run starts — the TUI
// reads it once to size the position bar.
func (p *ReplayProvider) Duration() time.Duration {
	p.ensureDecoded()
	return p.total
}

// Seek requests a jump to data-timeline position target (clamped to
// [0, Duration]). The jump is applied at the next frame boundary in Run: the
// pacing loop repositions the frame index and re-anchors so playback continues
// from there at the current speed with no catch-up rush. A backward seek
// re-emits the earlier frames. Safe to call concurrently with Run. No-op when
// Speed is 0 (unpaced), consistent with SetPaused/SetSpeed being inert there.
func (p *ReplayProvider) Seek(target time.Duration) {
	if p.Speed <= 0 {
		return
	}
	p.ensureDecoded()
	if target < 0 {
		target = 0
	}
	if target > p.total {
		target = p.total
	}
	p.mu.Lock()
	p.seekTo = &target
	p.mu.Unlock()
}

// PendingSeek reports a requested-but-not-yet-applied seek target (mirrors
// Paused/CurrentSpeed as a read accessor; used by the TUI's tests to confirm a
// key issued the seek it should).
func (p *ReplayProvider) PendingSeek() (time.Duration, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.seekTo == nil {
		return 0, false
	}
	return *p.seekTo, true
}

// takeSeek returns a pending seek target once, clearing it.
func (p *ReplayProvider) takeSeek() (time.Duration, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.seekTo == nil {
		return 0, false
	}
	t := *p.seekTo
	p.seekTo = nil
	return t, true
}

// seekIndex returns the first frame index whose Elapsed is >= target, clamped to
// a valid index. Frames are monotonic in ByteOffset, so a binary search suffices.
func seekIndex(frames []decoder.Frame, target time.Duration) int {
	lo, hi := 0, len(frames)
	for lo < hi {
		mid := (lo + hi) / 2
		if frameElapsed(frames[mid]) < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo >= len(frames) {
		lo = len(frames) - 1
	}
	return lo
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

	p.ensureDecoded()
	frames := p.frames
	if p.Speed <= 0 {
		// Unpaced: emit as fast as possible; runtime controls (incl. seek) are inert.
		for i, f := range frames {
			if err := ctx.Err(); err != nil {
				return err
			}
			emit(FrameEvent{Frame: f, Index: i, Elapsed: frameElapsed(f)})
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
	for i := 0; i < len(frames); i++ {
		f := frames[i]
		// Each capture byte is one ALDL bit at 160 bps, so a frame's byte
		// offset maps directly to its position in the original recording.
		// This is what the consumer sees as Elapsed — the data timeline, not
		// how long playback has been running — so it is independent of Speed.
		dataElapsed := frameElapsed(f)
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			// A pending seek jumps the frame index and re-anchors, then emits the
			// target frame immediately (even while paused, so a scrub shows the
			// frame under the playhead) — playback then holds or paces from there.
			if target, ok := p.takeSeek(); ok {
				i = seekIndex(frames, target)
				f = frames[i]
				dataElapsed = frameElapsed(f)
				anchorData = dataElapsed
				anchorWall = now()
				break
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
