package stream

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"goaldl/pkg/decoder"
)

// fastTCP returns a TCPProvider tuned so tests run in milliseconds, not the
// multi-second production defaults.
func fastTCP(addr string) *TCPProvider {
	return &TCPProvider{
		Addr:        addr,
		Config:      decoder.DefaultConfig(),
		DialTimeout: 200 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
		sleep: func(ctx context.Context, _ time.Duration) error {
			return ctxSleep(ctx, time.Millisecond)
		},
	}
}

// replayTCPServer serves data to the first accepted connection, closes it,
// and then stops listening — exactly one replay, so a frame-count oracle can't
// be inflated by the provider redialing for another pass. It doubles as the
// manual bench tool pattern for bridge bring-up (a paced variant would feed
// ~160 B/s); kept test-only per the spec (§7).
func replayTCPServer(t *testing.T, data []byte) (addr string, done func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Write(data)
		conn.Close()
	})
	return ln.Addr().String(), func() { ln.Close(); wg.Wait() }
}

// decodeAll is the oracle: the frames the decoder yields from data directly.
func decodeAll(cfg decoder.Config, data []byte) []decoder.Frame {
	d := decoder.New(cfg)
	var frames []decoder.Frame
	for _, b := range data {
		if f := d.Feed(b); f != nil {
			frames = append(frames, *f)
		}
	}
	return frames
}

func driveFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "decoder", "testdata", "drive_4800.raw"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return raw
}

// T1 + T2: a TCP source streaming the drive fixture emits exactly the frames
// the decoder yields from the fixture directly, and the Sink tee receives the
// byte stream byte-for-byte (a bridge session's .raw is interchangeable with a
// serial capture).
func TestTCPHappyPathAndSinkFidelity(t *testing.T) {
	raw := driveFixture(t)
	addr, done := replayTCPServer(t, raw)
	defer done()

	want := decodeAll(decoder.DefaultConfig(), raw)
	var sink bytes.Buffer
	p := fastTCP(addr)
	p.Sink = &sink

	ctx, cancel := context.WithCancel(context.Background())
	var got []decoder.Frame
	var mu sync.Mutex
	go func() {
		// The server closes after one replay; stop once everything arrived (the
		// provider itself never gives up, so the test ends the run).
		deadline := time.After(30 * time.Second)
		for {
			mu.Lock()
			n := len(got)
			enough := int64(len(raw)) <= p.Bytes()
			mu.Unlock()
			if n >= len(want) && enough {
				cancel()
				return
			}
			select {
			case <-deadline:
				cancel()
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}()
	err := p.Run(ctx, func(ev FrameEvent) {
		mu.Lock()
		got = append(got, ev.Frame)
		mu.Unlock()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	if len(got) != len(want) {
		t.Fatalf("emitted %d frames, want %d (fixture decoded directly)", len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(got[i].Data, want[i].Data) {
			t.Fatalf("frame %d differs from direct decode", i)
		}
	}
	if !bytes.Equal(sink.Bytes(), raw) {
		t.Errorf("sink got %d bytes, want the %d fixture bytes byte-for-byte", sink.Len(), len(raw))
	}
	if p.Bytes() != int64(len(raw)) {
		t.Errorf("Bytes() = %d, want %d", p.Bytes(), len(raw))
	}
}

// T3: a dropped connection mid-stream does not end the session — the provider
// redials, Reconnecting() toggles, and frames resume after the gap.
func TestTCPReconnectOnDrop(t *testing.T) {
	raw := driveFixture(t)
	half := len(raw) / 2
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			if i == 0 {
				conn.Write(raw[:half]) // partial stream, then drop
			} else {
				conn.Write(raw) // full replay on the redialed connection
			}
			conn.Close()
		}
	})

	// Frames from the full replay alone — the post-reconnect oracle. After the
	// drop the decoder restarts and resyncs, so at minimum these must arrive.
	wantAfter := len(decodeAll(decoder.DefaultConfig(), raw))

	p := fastTCP(ln.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	frames := 0
	var sawReconnecting bool
	// Every (re)dial goes through the injectable, so observe Reconnecting()
	// there — the redial after the drop is a dial with frames already seen.
	p.dial = func(dctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
		mu.Lock()
		if frames > 0 && p.Reconnecting() {
			sawReconnecting = true
		}
		mu.Unlock()
		d := net.Dialer{Timeout: timeout}
		return d.DialContext(dctx, "tcp", addr)
	}
	go func() {
		deadline := time.After(30 * time.Second)
		for {
			mu.Lock()
			n := frames
			mu.Unlock()
			if n >= wantAfter {
				cancel()
				return
			}
			select {
			case <-deadline:
				cancel()
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}()
	err = p.Run(ctx, func(FrameEvent) {
		mu.Lock()
		frames++
		mu.Unlock()
	})
	ln.Close() // unblock the accept loop before waiting on it
	wg.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if frames < wantAfter {
		t.Errorf("only %d frames arrived, want ≥%d (stream must resume after the drop)", frames, wantAfter)
	}
	if !sawReconnecting {
		t.Error("Reconnecting() was never observed true during the gap")
	}
	if p.Reconnecting() || p.ReconnectAttempt() != 0 {
		t.Error("reconnect state should reset after Run returns")
	}
}

// T4: cancelling the context returns promptly even while a Read is blocked on
// a silent connection — the cancel-closer goroutine unblocks it.
func TestTCPContextCancelUnblocksRead(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			// accept and hold silently: the provider's Read blocks
		}
	}()

	p := fastTCP(ln.Addr().String())
	p.ReadTimeout = 10 * time.Second // force reliance on the cancel-closer, not the deadline

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx, func(FrameEvent) {}) }()

	time.Sleep(50 * time.Millisecond) // let it connect and block in Read
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
		if lat := time.Since(start); lat > time.Second {
			t.Errorf("cancel latency %v, want well under the 10s read deadline", lat)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel — blocked Read was never unblocked")
	}
}

// T5: a half-open socket (connected but eternally silent, no FIN) trips the
// empty-window threshold and redials within ~tcpHalfOpenWindows·ReadTimeout,
// rather than hanging forever.
func TestTCPHalfOpenRedials(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	var mu sync.Mutex
	accepts := 0
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			accepts++
			mu.Unlock()
			defer conn.Close() // never write, never close — half-open
		}
	}()

	p := fastTCP(ln.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx, func(FrameEvent) {}) }()

	// Upper bound: a few windows' worth. With ReadTimeout=50ms and 4 windows,
	// a redial should happen well within 2s.
	deadline := time.After(10 * time.Second)
	for {
		mu.Lock()
		n := accepts
		mu.Unlock()
		if n >= 2 {
			break // it gave up on the silent conn and dialed again
		}
		select {
		case <-deadline:
			t.Fatal("provider never redialed a half-open connection")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
}

// T6: no listener at Addr at launch is not fatal — the provider stays in the
// redial loop and connects when the endpoint appears.
func TestTCPDialRefusedThenConnects(t *testing.T) {
	// Reserve an address, then close it so the first dials are refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	p := fastTCP(addr)
	var mu sync.Mutex
	var sawReconnecting bool
	attempts := 0
	started := make(chan struct{})
	p.sleep = func(c context.Context, _ time.Duration) error {
		mu.Lock()
		if p.Reconnecting() {
			sawReconnecting = true
		}
		attempts++
		if attempts == 3 {
			close(started) // a few refusals in, bring the listener up
		}
		mu.Unlock()
		return ctxSleep(c, time.Millisecond)
	}

	ctx, cancel := context.WithCancel(context.Background())
	frames := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- p.Run(ctx, func(FrameEvent) {
			select {
			case frames <- struct{}{}:
			default:
			}
		})
	}()

	<-started
	ln2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("relisten on %s: %v", addr, err)
	}
	defer ln2.Close()
	raw := driveFixture(t)
	go func() {
		for {
			conn, err := ln2.Accept()
			if err != nil {
				return
			}
			conn.Write(raw)
			conn.Close()
		}
	}()

	select {
	case <-frames:
		// connected and decoding after the endpoint appeared
	case <-time.After(30 * time.Second):
		t.Fatal("no frame arrived after the listener came up")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	if !sawReconnecting {
		t.Error("Reconnecting() should be true while dialing an absent endpoint")
	}
}

// Sibling of TestSerialReconnectNeverGivesUp: the redial loop keeps retrying
// until the endpoint returns or the context is cancelled — it never
// self-terminates, so a long bridge outage doesn't end the session.
func TestTCPRedialNeverGivesUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dials := 0
	p := fastTCP("192.0.2.1:1") // TEST-NET, never dialed for real
	p.dial = func(context.Context, string, time.Duration) (net.Conn, error) {
		dials++
		if dials > 100 { // still trying well past any old give-up cap
			cancel()
		}
		return nil, errors.New("still gone")
	}
	err := p.Run(ctx, func(FrameEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled (redial must never self-give-up)", err)
	}
	if dials <= 100 {
		t.Errorf("dial attempted %d times, want it to keep retrying past 100", dials)
	}
	if p.Reconnecting() || p.ReconnectAttempt() != 0 {
		t.Error("reconnect state should reset after Run returns")
	}
}

// T7: diagnostics are safe to read concurrently with Run (exercised under
// -race) and Bytes() climbs as data arrives.
func TestTCPDiagnosticsRace(t *testing.T) {
	raw := driveFixture(t)
	addr, done := replayTCPServer(t, raw)
	defer done()

	p := fastTCP(addr)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- p.Run(ctx, func(FrameEvent) {}) }()

	deadline := time.After(30 * time.Second)
	for p.Bytes() == 0 {
		p.Reconnecting()
		p.ReconnectAttempt()
		select {
		case <-deadline:
			t.Fatal("Bytes() never climbed")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	if err := <-runDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
}

// T8: Name() is the configured address, stable regardless of connection state.
func TestTCPName(t *testing.T) {
	p := &TCPProvider{Addr: "bridge.local:3333"}
	if got := p.Name(); got != "tcp:bridge.local:3333" {
		t.Errorf("Name() = %q, want %q", got, "tcp:bridge.local:3333")
	}
}
