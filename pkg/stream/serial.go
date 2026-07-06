package stream

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	"goaldl/pkg/decoder"
	"goaldl/pkg/serial"
)

// serialPort is the minimal port behavior SerialProvider needs — satisfied by
// *serial.AldlSerial. An interface so the reconnect loop can be exercised with a
// fake port in tests (there is no real serial hardware in CI).
type serialPort interface {
	Read(buf []byte) (int, error)
	ResetInputBuffer() error
	Close() error
}

// SerialProvider streams frames decoded live from an ECM over a serial port.
// If Sink is non-nil, every raw byte read is also written to it, so a live
// session can be recorded to a capture file at the same time it is displayed.
//
// A missing port never ends the session. Both the initial open and a
// mid-session read failure (cable unplugged, adapter reset) drop into a
// reconnect loop (see reconnect) that retries until the port comes back or the
// context is cancelled — so the dashboard keeps its accumulated state across an
// outage and resumes the moment the cable returns. The consumer distinguishes
// "connecting" from "reconnecting after data" via Reconnecting()/ReconnectAttempt
// and whether it has yet seen a frame.
type SerialProvider struct {
	Port   string
	Baud   int
	Config decoder.Config
	Sink   io.Writer // optional: raw capture tee

	nbytes        atomic.Int64 // total raw bytes read (waiting-screen diagnostics; see Bytes)
	reconnecting  atomic.Bool  // true while Run is retrying a dropped connection
	reconnAttempt atomic.Int64 // current reconnect attempt (0 when not reconnecting)

	// open/sleep/listPorts are injectable for tests; nil uses the real ones.
	open      func(port string, baud int) (serialPort, error)
	sleep     func(ctx context.Context, d time.Duration) error
	listPorts func() ([]string, error)
}

func (p *SerialProvider) Name() string { return "live:" + p.Port }

// Bytes returns the total raw bytes read from the port so far. The waiting
// screen uses it to tell "no bytes at all" (cable/port/driver) from "bytes but
// no frame sync" (baud/polarity). Safe to read concurrently with Run.
func (p *SerialProvider) Bytes() int64 { return p.nbytes.Load() }

// Reconnecting reports whether Run is currently retrying a dropped connection.
// The dashboard shows a reconnecting indicator (rather than the fatal panel)
// while this holds. Safe to read concurrently with Run.
func (p *SerialProvider) Reconnecting() bool { return p.reconnecting.Load() }

// ReconnectAttempt returns the current reconnect attempt number (0 when not
// reconnecting), for the dashboard's "reconnecting N/max" indicator. Safe to
// read concurrently with Run.
func (p *SerialProvider) ReconnectAttempt() int { return int(p.reconnAttempt.Load()) }

// Reconnect tuning: retry once a second (about the frame cadence); after a few
// misses on the launch port name, rescan for a re-enumerated device (a macOS
// PL2303 can come back under a different /dev name).
const (
	reconnectInterval    = time.Second
	reconnectRescanAfter = 3
)

func (p *SerialProvider) opener() func(string, int) (serialPort, error) {
	if p.open != nil {
		return p.open
	}
	return func(port string, baud int) (serialPort, error) { return serial.NewWithBaudRate(port, baud) }
}

func (p *SerialProvider) sleeper() func(context.Context, time.Duration) error {
	if p.sleep != nil {
		return p.sleep
	}
	return ctxSleep
}

func (p *SerialProvider) Run(ctx context.Context, emit func(FrameEvent)) error {
	open := p.opener()
	sleep := p.sleeper()

	curName := p.Port
	ser, err := open(curName, p.Baud)
	if err != nil {
		// No port at launch (cable not in yet, or a name that will appear): wait
		// for it rather than failing — the dashboard shows a waiting screen.
		ser, curName, err = p.reconnect(ctx, curName, open, sleep)
		if err != nil {
			return err // only ctx cancellation escapes the reconnect loop
		}
	} else if rerr := ser.ResetInputBuffer(); rerr != nil {
		ser.Close()
		ser, curName, err = p.reconnect(ctx, curName, open, sleep)
		if err != nil {
			return err
		}
	}
	defer func() { ser.Close() }()

	d := decoder.New(p.Config)
	buf := make([]byte, 512)
	start := time.Now()
	idx := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := ser.Read(buf)
		if err != nil {
			// The port dropped mid-session. Keep the session alive and reconnect
			// rather than ending it — a bumped cable at the car must recover.
			ser.Close()
			var newSer serialPort
			newSer, curName, err = p.reconnect(ctx, curName, open, sleep)
			if err != nil {
				return err // only ctx cancellation escapes the reconnect loop
			}
			ser = newSer
			d = decoder.New(p.Config) // resync from scratch after the gap
			continue
		}
		if n == 0 {
			continue // read timeout, no data yet
		}
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
}

// reconnect retries opening the port until it succeeds or ctx is cancelled — it
// never gives up on its own, so a dropped or not-yet-present cable recovers the
// moment it returns and the session (and its accumulated grids) survives. It
// tries curName first; after reconnectRescanAfter misses it rescans for a single
// present USB serial port (macOS re-enumeration can rename the device) and, if
// exactly one is found, tries that instead. Returns the (re)opened port and the
// name that worked, so a subsequent drop retries the name last seen good. The
// attempt counter (ReconnectAttempt) climbs for the dashboard's indicator.
func (p *SerialProvider) reconnect(ctx context.Context, curName string, open func(string, int) (serialPort, error), sleep func(context.Context, time.Duration) error) (serialPort, string, error) {
	p.reconnecting.Store(true)
	defer func() {
		p.reconnecting.Store(false)
		p.reconnAttempt.Store(0)
	}()
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, curName, err
		}
		p.reconnAttempt.Store(int64(attempt))
		name := curName
		if attempt > reconnectRescanAfter {
			if found := p.singlePort(); found != "" {
				name = found
			}
		}
		if ser, err := open(name, p.Baud); err == nil {
			if rerr := ser.ResetInputBuffer(); rerr == nil {
				return ser, name, nil
			}
			ser.Close()
		}
		if err := sleep(ctx, reconnectInterval); err != nil {
			return nil, curName, err
		}
	}
}

// singlePort returns the sole available USB serial port, or "" when zero or
// several are present (ambiguous — keep retrying the known name instead).
func (p *SerialProvider) singlePort() string {
	list := p.listPorts
	if list == nil {
		list = serial.AvailablePorts
	}
	if ports, err := list(); err == nil && len(ports) == 1 {
		return ports[0]
	}
	return ""
}
