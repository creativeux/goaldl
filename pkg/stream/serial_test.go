package stream

import (
	"context"
	"errors"
	"testing"
	"time"

	"goaldl/pkg/decoder"
)

// fakePort is a scripted serialPort: each Read pops the next payload; once the
// script is exhausted (or a nil entry is hit) Read returns an error, which
// SerialProvider.Run treats as a mid-session disconnect.
type fakePort struct {
	reads  [][]byte
	i      int
	closed int
}

func (f *fakePort) Read(b []byte) (int, error) {
	if f.i >= len(f.reads) || f.reads[f.i] == nil {
		f.i++
		return 0, errors.New("read: disconnected")
	}
	n := copy(b, f.reads[f.i])
	f.i++
	return n, nil
}
func (f *fakePort) ResetInputBuffer() error { return nil }
func (f *fakePort) Close() error            { f.closed++; return nil }

// TestSerialReconnect: a mid-session read failure does not end the session — Run
// retries (on the launch name, then a rescanned name after a few misses),
// resumes streaming when the port returns, keeps Reconnecting() true across the
// gap, and only exits when the context is cancelled. The initial open failing
// stays terminal.
func TestSerialReconnect(t *testing.T) {
	port1 := &fakePort{reads: [][]byte{{0xAA, 0xBB}}}       // 2 bytes, then disconnect
	port2 := &fakePort{reads: [][]byte{{0xCC, 0xDD, 0xEE}}} // 3 bytes after reconnect

	ctx, cancel := context.WithCancel(context.Background())
	var opens []string
	var sawReconnecting bool
	var p *SerialProvider
	p = &SerialProvider{
		Port: "/dev/orig", Baud: 4800, Config: decoder.DefaultConfig(),
		open: func(name string, _ int) (serialPort, error) {
			opens = append(opens, name)
			switch len(opens) {
			case 1:
				return port1, nil // initial open succeeds
			case 2, 3, 4:
				return nil, errors.New("still gone") // attempts 1–3 fail (launch name)
			case 5:
				return port2, nil // attempt 4 (> rescanAfter) recovers on the rescanned name
			default:
				cancel() // recovery consumed — end the run
				return nil, errors.New("stop")
			}
		},
		sleep: func(c context.Context, _ time.Duration) error {
			if p.Reconnecting() {
				sawReconnecting = true
			}
			return c.Err()
		},
		listPorts: func() ([]string, error) { return []string{"/dev/new"}, nil },
	}

	err := p.Run(ctx, func(FrameEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	if !sawReconnecting {
		t.Error("Reconnecting() was never true during the gap")
	}
	if len(opens) < 5 {
		t.Fatalf("opener called %d times, want ≥5 (initial + retries + recovery)", len(opens))
	}
	// attempts 1–3 (opens[1..3]) retry the launch name; attempt 4 (opens[4]) is
	// past reconnectRescanAfter, so it adopts the single rescanned port.
	if opens[1] != "/dev/orig" {
		t.Errorf("first retry used %q, want the launch name /dev/orig", opens[1])
	}
	if opens[4] != "/dev/new" {
		t.Errorf("post-rescan retry used %q, want the rescanned /dev/new", opens[4])
	}
	if p.Bytes() != 5 {
		t.Errorf("Bytes() = %d, want 5 (2 before + 3 after reconnect)", p.Bytes())
	}
	if port1.closed == 0 {
		t.Error("the dropped port was not closed before reconnecting")
	}
	if p.Reconnecting() {
		t.Error("Reconnecting() should be false after Run returns")
	}
}

// TestSerialReconnectNeverGivesUp: reconnect keeps retrying until the port
// returns or the context is cancelled — it never self-terminates, so a long
// outage doesn't end the session (the dashboard keeps its accumulated grids).
func TestSerialReconnectNeverGivesUp(t *testing.T) {
	port1 := &fakePort{reads: [][]byte{{0xAA}}} // one read, then a permanent drop
	var openCalls int
	ctx, cancel := context.WithCancel(context.Background())
	p := &SerialProvider{
		Port: "/dev/orig", Baud: 4800, Config: decoder.DefaultConfig(),
		open: func(string, int) (serialPort, error) {
			openCalls++
			switch {
			case openCalls == 1:
				return port1, nil
			case openCalls > 100: // still trying well past any old give-up cap
				cancel()
				return nil, errors.New("stop")
			default:
				return nil, errors.New("still gone")
			}
		},
		sleep:     func(c context.Context, _ time.Duration) error { return c.Err() },
		listPorts: func() ([]string, error) { return nil, nil },
	}
	err := p.Run(ctx, func(FrameEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled (reconnect must never self-give-up)", err)
	}
	if openCalls <= 30 {
		t.Errorf("opener called %d times, want it to keep retrying well past 30", openCalls)
	}
	if p.Reconnecting() || p.ReconnectAttempt() != 0 {
		t.Error("reconnect state should reset after Run returns")
	}
}

// TestSerialInitialOpenWaits: the initial open failing is no longer terminal —
// Run waits (reconnects) for the port to appear rather than erroring, and
// streams once it does. Reconnecting() is true during the wait.
func TestSerialInitialOpenWaits(t *testing.T) {
	port := &fakePort{reads: [][]byte{{0x11, 0x22}}}
	var openCalls int
	var sawReconnecting bool
	ctx, cancel := context.WithCancel(context.Background())
	var p *SerialProvider
	p = &SerialProvider{
		Port: "/dev/late", Baud: 4800, Config: decoder.DefaultConfig(),
		open: func(string, int) (serialPort, error) {
			openCalls++
			switch {
			case openCalls < 3:
				return nil, errors.New("not plugged in yet") // absent at launch
			case openCalls == 3:
				return port, nil // appears on the 3rd try
			default:
				cancel() // after the port drops again, end the run
				return nil, errors.New("stop")
			}
		},
		sleep: func(c context.Context, _ time.Duration) error {
			if p.Reconnecting() {
				sawReconnecting = true
			}
			return c.Err()
		},
		listPorts: func() ([]string, error) { return nil, nil },
	}
	err := p.Run(ctx, func(FrameEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	if !sawReconnecting {
		t.Error("Reconnecting() should be true while waiting for the initial port")
	}
	if openCalls < 4 {
		t.Errorf("opener called %d times, want ≥4 (2 misses + open + post-drop)", openCalls)
	}
	if p.Bytes() != 2 {
		t.Errorf("Bytes() = %d, want 2 (streamed after the port appeared)", p.Bytes())
	}
}
