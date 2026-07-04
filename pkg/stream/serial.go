package stream

import (
	"context"
	"io"
	"time"

	"goaldl/pkg/decoder"
	"goaldl/pkg/serial"
)

// SerialProvider streams frames decoded live from an ECM over a serial port.
// If Sink is non-nil, every raw byte read is also written to it, so a live
// session can be recorded to a capture file at the same time it is displayed.
type SerialProvider struct {
	Port   string
	Baud   int
	Config decoder.Config
	Sink   io.Writer // optional: raw capture tee
}

func (p *SerialProvider) Name() string { return "live:" + p.Port }

func (p *SerialProvider) Run(ctx context.Context, emit func(FrameEvent)) error {
	ser, err := serial.NewWithBaudRate(p.Port, p.Baud)
	if err != nil {
		return err
	}
	defer ser.Close()
	if err := ser.ResetInputBuffer(); err != nil {
		return err
	}

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
			return err
		}
		if n == 0 {
			continue // read timeout, no data yet
		}
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
