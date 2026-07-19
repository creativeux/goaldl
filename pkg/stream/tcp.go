package stream

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync/atomic"
	"time"

	"goaldl/pkg/decoder"
)

// TCPProvider streams frames decoded live from a TCP byte source — an ESP32
// bridge forwarding the raw ALDL UART stream over WiFi or wired Ethernet. It is
// the network twin of SerialProvider: the bridge hands us the same 0xFE/0x00
// byte values a local adapter would, so the decoder Config and the whole
// downstream pipeline are identical; only the byte transport differs.
//
// If Sink is non-nil, every received byte is also written to it, so a bridge
// session records to a .raw that is byte-for-byte identical to a serial capture.
//
// A dropped connection never ends the session. Both the initial dial and a
// mid-session read failure (bridge reboot, WiFi blip, cable bump) drop into a
// redial loop that retries until the bridge comes back or the context is
// cancelled — so the dashboard keeps its accumulated grids across an outage and
// resumes when the bridge returns. The consumer distinguishes "connecting" from
// "reconnecting after data" via Reconnecting()/ReconnectAttempt() and whether it
// has yet seen a frame.
type TCPProvider struct {
	Addr   string         // "host:port" (dialed with net; IPv6 literals use [::1]:port form)
	Config decoder.Config // same baud model/polarity/thresholds as a serial session
	Sink   io.Writer      // optional: raw capture tee

	// Timeouts (zero ⇒ package defaults). Injectable so tests run fast.
	DialTimeout time.Duration // per-dial-attempt ceiling
	ReadTimeout time.Duration // rolling read deadline for half-open/liveness detection

	nbytes        atomic.Int64 // total raw bytes received (waiting-screen diagnostics; see Bytes)
	reconnecting  atomic.Bool  // true while Run is redialing a dropped connection
	reconnAttempt atomic.Int64 // current redial attempt (0 when connected)

	// dial/sleep are injectable for tests; nil uses the real ones.
	dial  func(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error)
	sleep func(ctx context.Context, d time.Duration) error
}

func (p *TCPProvider) Name() string { return "tcp:" + p.Addr }

// Bytes returns the total raw bytes received from the bridge so far. The
// waiting screen uses it to tell "no bytes at all" (bridge down/unreachable)
// from "bytes but no frame sync" (baud/polarity/wiring at the bridge's UART).
// Safe to read concurrently with Run.
func (p *TCPProvider) Bytes() int64 { return p.nbytes.Load() }

// Reconnecting reports whether Run is currently redialing a dropped
// connection. The dashboard shows a reconnecting indicator (rather than the
// fatal panel) while this holds. Safe to read concurrently with Run.
func (p *TCPProvider) Reconnecting() bool { return p.reconnecting.Load() }

// ReconnectAttempt returns the current redial attempt number (0 when
// connected), for the dashboard's reconnecting indicator. Safe to read
// concurrently with Run.
func (p *TCPProvider) ReconnectAttempt() int { return int(p.reconnAttempt.Load()) }

// TCP liveness tuning. A healthy bridge delivers a frame's worth of bytes
// about every 1.2s, so a 3s window without a single byte is anomalous but not
// trigger-happy; after tcpHalfOpenWindows such windows in a row we assume a
// half-open socket (bridge powered off without a FIN) and redial. These gate
// the connection only — never frame content (raw-data policy).
const (
	defaultTCPDialTimeout = 5 * time.Second
	defaultTCPReadTimeout = 3 * time.Second
	tcpHalfOpenWindows    = 4
)

func (p *TCPProvider) dialer() func(context.Context, string, time.Duration) (net.Conn, error) {
	if p.dial != nil {
		return p.dial
	}
	return func(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
		d := net.Dialer{Timeout: timeout}
		return d.DialContext(ctx, "tcp", addr)
	}
}

func (p *TCPProvider) sleeper() func(context.Context, time.Duration) error {
	if p.sleep != nil {
		return p.sleep
	}
	return ctxSleep
}

func (p *TCPProvider) dialTimeout() time.Duration {
	if p.DialTimeout > 0 {
		return p.DialTimeout
	}
	return defaultTCPDialTimeout
}

func (p *TCPProvider) readTimeout() time.Duration {
	if p.ReadTimeout > 0 {
		return p.ReadTimeout
	}
	return defaultTCPReadTimeout
}

func (p *TCPProvider) Run(ctx context.Context, emit func(FrameEvent)) error {
	conn, err := p.redial(ctx)
	if err != nil {
		return err // only ctx cancellation escapes the redial loop
	}

	// A blocked Read does not return on ctx cancel by itself. The rolling read
	// deadline bounds the wait, and this closer makes cancel latency ~0: it
	// closes the *current* conn the moment ctx is done. It is re-armed for each
	// connection (the channel swap keeps it off a stale conn during a redial),
	// and stops when Run returns for a non-ctx reason (e.g. a Sink write error)
	// so it never outlives the provider.
	connCh := make(chan net.Conn, 1)
	connCh <- conn
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-stop:
			return
		case <-ctx.Done():
		}
		if c := <-connCh; c != nil {
			c.Close()
		}
	}()
	// swapConn hands the closer the new current conn (nil while between conns).
	// It never blocks: either the closer holds the slot (post-cancel; close the
	// new conn ourselves) or we own it.
	swapConn := func(c net.Conn) {
		select {
		case old := <-connCh:
			_ = old
		default:
		}
		select {
		case connCh <- c:
		case <-ctx.Done():
			if c != nil {
				c.Close()
			}
		}
	}
	defer func() {
		swapConn(nil)
		if conn != nil {
			conn.Close()
		}
	}()

	d := decoder.New(p.Config)
	buf := make([]byte, 512)
	start := time.Now()
	idx := 0
	emptyWindows := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn.SetReadDeadline(time.Now().Add(p.readTimeout()))
		n, err := conn.Read(buf)
		if n > 0 {
			// Process received bytes before looking at err, so trailing bytes
			// delivered alongside an EOF still reach the sink and decoder.
			emptyWindows = 0
			p.nbytes.Add(int64(n))
			if p.Sink != nil {
				if _, werr := p.Sink.Write(buf[:n]); werr != nil {
					return werr
				}
			}
			for _, b := range buf[:n] {
				if f := d.Feed(b); f != nil {
					emit(FrameEvent{Frame: *f, Index: idx, Elapsed: time.Since(start)})
					idx++
				}
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, os.ErrDeadlineExceeded) {
			if n > 0 {
				continue
			}
			// No data this window — still waiting, unless the silence has gone
			// on long enough to mean a half-open socket; then redial.
			emptyWindows++
			if emptyWindows < tcpHalfOpenWindows {
				continue
			}
		}
		// The bridge dropped (or went silent past the half-open threshold).
		// Keep the session alive and redial rather than ending it.
		swapConn(nil)
		conn.Close()
		conn, err = p.redial(ctx)
		if err != nil {
			return err // only ctx cancellation escapes the redial loop
		}
		swapConn(conn)
		d = decoder.New(p.Config) // resync from scratch after the gap
		emptyWindows = 0
	}
}

// redial retries dialing Addr until it succeeds or ctx is cancelled — it never
// gives up on its own, so a bridge that reboots or a WiFi blip recovers the
// moment the endpoint returns and the session (and its accumulated grids)
// survives. Unlike serial's reconnect there is no port rescan: a TCP endpoint
// does not rename itself, so we always redial the same Addr. The attempt
// counter (ReconnectAttempt) climbs for the dashboard's indicator.
func (p *TCPProvider) redial(ctx context.Context) (net.Conn, error) {
	dial := p.dialer()
	sleep := p.sleeper()
	p.reconnecting.Store(true)
	defer func() {
		p.reconnecting.Store(false)
		p.reconnAttempt.Store(0)
	}()
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.reconnAttempt.Store(int64(attempt))
		if conn, err := dial(ctx, p.Addr, p.dialTimeout()); err == nil {
			return conn, nil
		}
		if err := sleep(ctx, reconnectInterval); err != nil {
			return nil, err
		}
	}
}
