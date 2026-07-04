package stream

import (
	"context"

	"goaldl/pkg/aldl"
	"goaldl/pkg/ecm"
)

// Snapshot is one frame's fully-processed state — the unit a front-end
// consumes. It composes the raw frame event with the parsed sensor values,
// fuel-trim state, and PROM check, so a consumer (TUI, an HTTP/WebSocket
// server, a mobile bridge, …) never has to touch the decoder or ecm packages
// directly. Every field is plain data, so a Snapshot serializes cleanly.
type Snapshot struct {
	FrameEvent                    // frame, index, elapsed
	PROMOK     bool               // frame's PROM ID matched the expected one
	Sensors    map[string]float64 // parsed engineering values by parameter name
	FuelTrim   ecm.FuelTrim       // RPM/MAP/BLM + closed-loop gating for BLM work
}

// Session is the core engine: it drives a Provider (live serial or a replay),
// decodes and parses each frame, and emits a Snapshot per frame. It is the
// single API a system of engagement builds on — the TUI is one consumer; a web
// or mobile front-end would consume the same Snapshot stream.
type Session struct {
	provider Provider
	registry *ecm.Registry
	ecmPart  string
	promID   int
}

// NewSession builds a session over an already-constructed provider (serial or
// replay). promID is the expected PROM for the per-frame PROMOK flag (0 to
// disable the check).
func NewSession(provider Provider, registry *ecm.Registry, ecmPart string, promID int) *Session {
	return &Session{provider: provider, registry: registry, ecmPart: ecmPart, promID: promID}
}

// Name returns the underlying provider's name (e.g. "replay", "live:/dev/…").
func (s *Session) Name() string { return s.provider.Name() }

// Run drives the session until the source is exhausted, ctx is cancelled, or an
// error occurs, delivering a Snapshot per frame to emit. emit is called on the
// provider's goroutine; a consumer that shares state across goroutines must
// synchronize (the snapshot itself is a fresh value per frame).
func (s *Session) Run(ctx context.Context, emit func(Snapshot)) error {
	return s.provider.Run(ctx, func(ev FrameEvent) {
		emit(s.snapshot(ev))
	})
}

func (s *Session) snapshot(ev FrameEvent) Snapshot {
	sensors := map[string]float64{}
	if data, err := s.registry.ParseFrame(&aldl.Frame{Data: ev.Frame.Data}, s.ecmPart); err == nil {
		sensors = data.ParsedValues
	}
	return Snapshot{
		FrameEvent: ev,
		PROMOK:     s.promID == 0 || promOf(ev.Frame.Data) == s.promID,
		Sensors:    sensors,
		FuelTrim:   ecm.FuelTrimSample(ev.Frame.Data),
	}
}

// promOf reads the 16-bit PROM ID from a frame (bytes 1-2), or -1 if too short.
func promOf(data []byte) int {
	if len(data) < 3 {
		return -1
	}
	return int(data[1])<<8 | int(data[2])
}
