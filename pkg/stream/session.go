package stream

import (
	"context"

	"goaldl/pkg/ecm"
)

// Snapshot is one frame's fully-processed state — the unit a front-end
// consumes. It composes the raw frame event with the parsed sensor values,
// decoded flags and trouble codes, fuel-trim state, and PROM check, so a
// consumer (TUI, an HTTP/WebSocket server, a mobile bridge, …) never has to
// touch the decoder or ecm packages directly. Every field is plain data, so a
// Snapshot serializes cleanly.
type Snapshot struct {
	FrameEvent                      // frame, index, elapsed
	PROMOK     bool                 // frame's PROM ID matched the expected one
	ParseOK    bool                 // sensors were parsed successfully (else Sensors is empty)
	Sensors    map[string]float64   // parsed engineering values by parameter name
	FuelTrim   ecm.FuelTrim         // RPM/MAP/BLM + closed-loop gating for BLM work
	Flags      []ecm.FlagWordStatus // decoded status-word bits (nil if frame too short)
	Codes      []ecm.CodeStatus     // decoded trouble codes, sorted (nil if frame too short)
}

// Session is the core engine: it drives a Provider (live serial or a replay),
// decodes and parses each frame, and emits a Snapshot per frame. It is the
// single API a system of engagement builds on — the TUI is one consumer; a web
// or mobile front-end would consume the same Snapshot stream.
type Session struct {
	provider Provider
	def      *ecm.Definition // nil when ecmPart is unknown → ParseOK stays false
	promID   int
}

// NewSession builds a session over an already-constructed provider (serial or
// replay). promID is the expected PROM for the per-frame PROMOK flag (0 to
// disable the check).
func NewSession(provider Provider, registry *ecm.Registry, ecmPart string, promID int) *Session {
	def, _ := registry.GetDefinition(ecmPart)
	return &Session{provider: provider, def: def, promID: promID}
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
	parseOK := false
	if s.def != nil {
		if parsed, err := s.def.Parse(ev.Frame.Data); err == nil {
			sensors, parseOK = parsed, true
		}
	}
	return Snapshot{
		FrameEvent: ev,
		PROMOK:     s.promID == 0 || ecm.FramePROM(ev.Frame.Data) == s.promID,
		ParseOK:    parseOK,
		Sensors:    sensors,
		FuelTrim:   ecm.FuelTrimSample(ev.Frame.Data),
		Flags:      ecm.DecodeFlags(s.def, ev.Frame.Data),
		Codes:      ecm.DecodeCodes(s.def, ev.Frame.Data),
	}
}
