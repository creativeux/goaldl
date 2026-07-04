package stream

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

// failWriter accepts okBytes bytes, then fails every subsequent write.
type failWriter struct {
	buf     bytes.Buffer
	okBytes int
}

func (f *failWriter) Write(p []byte) (int, error) {
	if f.buf.Len()+len(p) > f.okBytes {
		return 0, errors.New("disk full")
	}
	return f.buf.Write(p)
}

func TestRecordSinkDetachedDiscards(t *testing.T) {
	var s RecordSink
	n, err := s.Write([]byte("abcd"))
	if n != 4 || err != nil {
		t.Fatalf("detached Write = (%d, %v), want (4, nil)", n, err)
	}
	if s.Active() || s.Bytes() != 0 {
		t.Errorf("detached sink: Active=%v Bytes=%d, want false/0", s.Active(), s.Bytes())
	}
}

func TestRecordSinkAttachCountSwap(t *testing.T) {
	var s RecordSink
	var buf bytes.Buffer
	if old, n := s.Set(&buf); old != nil || n != 0 {
		t.Fatalf("first Set returned (%v, %d), want (nil, 0)", old, n)
	}
	s.Write([]byte("hello "))
	s.Write([]byte("world"))
	if !s.Active() || s.Bytes() != 11 {
		t.Errorf("Active=%v Bytes=%d, want true/11", s.Active(), s.Bytes())
	}
	old, n := s.Set(nil)
	if old != &buf || n != 11 {
		t.Errorf("Set(nil) = (%v, %d), want (&buf, 11)", old, n)
	}
	if buf.String() != "hello world" {
		t.Errorf("recorded %q", buf.String())
	}
	if s.Active() || s.Bytes() != 0 {
		t.Errorf("after stop: Active=%v Bytes=%d, want false/0", s.Active(), s.Bytes())
	}
}

// A target write error must detach the recording and keep the error for the
// consumer, while the provider-facing Write keeps reporting success — a dead
// log file must not kill the live session.
func TestRecordSinkWriteErrorDetaches(t *testing.T) {
	var s RecordSink
	fw := &failWriter{okBytes: 4}
	s.Set(fw)
	if n, err := s.Write([]byte("abcd")); n != 4 || err != nil {
		t.Fatalf("ok Write = (%d, %v)", n, err)
	}
	if n, err := s.Write([]byte("efgh")); n != 4 || err != nil {
		t.Fatalf("failing Write = (%d, %v), want success upward", n, err)
	}
	if s.Active() {
		t.Error("sink still active after target error")
	}
	if s.Err() == nil || s.Err().Error() != "disk full" {
		t.Errorf("Err() = %v, want disk full", s.Err())
	}
	// Later writes discard silently; error stays sticky until Set.
	s.Write([]byte("more"))
	if s.Err() == nil {
		t.Error("sticky error cleared by a discard write")
	}
	s.Set(nil)
	if s.Err() != nil {
		t.Error("Set did not clear the sticky error")
	}
}

// Concurrent Write/Set must be clean under -race (provider goroutine writes
// while the TUI toggles).
func TestRecordSinkConcurrent(t *testing.T) {
	var s RecordSink
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
				s.Write([]byte{0xFE, 0x00})
			}
		}
	})
	for range 100 {
		var buf bytes.Buffer
		s.Set(&buf)
		s.Bytes()
		s.Active()
		s.Set(nil)
	}
	close(done)
	wg.Wait()
}
