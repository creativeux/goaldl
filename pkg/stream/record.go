package stream

import (
	"io"
	"sync"
)

// RecordSink is an io.Writer whose target can be attached and detached while a
// SerialProvider is writing to it, so a live session can start and stop raw
// capture on demand (the TUI's `r` toggle). It is passed once as the
// provider's Sink at construction; when no target is set, writes are
// discarded.
//
// Write never returns an error to the provider: a target write error (disk
// full, pulled media) must stop the recording, not kill the live session.
// The error is kept (see Err) for the consumer to surface, and the target is
// detached. This is deliberately different from `monitor -o`, where recording
// was pre-declared and dying loudly is correct.
type RecordSink struct {
	mu  sync.Mutex
	w   io.Writer
	n   int64 // bytes written to the current target
	err error // sticky write error, cleared by Set
}

// Write forwards to the current target, counting bytes; with no target it
// discards. On a target error it detaches and records the error, but still
// reports success upward so the provider keeps streaming.
func (s *RecordSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w == nil {
		return len(p), nil
	}
	n, err := s.w.Write(p)
	s.n += int64(n)
	if err != nil {
		s.w, s.err = nil, err
	}
	return len(p), nil
}

// Set swaps the recording target (nil to stop), returning the previous target
// so the caller can close it, and the byte count written to it. The byte
// counter and any sticky error reset for the new target.
func (s *RecordSink) Set(w io.Writer) (old io.Writer, written int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, written = s.w, s.n
	s.w, s.n, s.err = w, 0, nil
	return old, written
}

// Active reports whether a target is currently attached.
func (s *RecordSink) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w != nil
}

// Bytes returns the byte count written to the current target.
func (s *RecordSink) Bytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// Err returns the sticky write error that detached the last target, if any.
// Cleared by the next Set.
func (s *RecordSink) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}
