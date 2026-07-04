package stream

import (
	"strings"
	"testing"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
)

// closedFrame builds a 20-byte frame at RPM 1600 (byte 64), MAP byte 99
// (~40 kPa), with the given MWAF1 and integrator/O2 bytes.
func gridFrame(mwaf1, intB, o2B byte) FrameEvent {
	f := make([]byte, 20)
	f[6], f[7] = 99, 64     // MAP, RPM
	f[9], f[10] = intB, o2B // INT, O2
	f[14] = mwaf1           // MWAF1 (bit7 closed, bit1 BLM-enable)
	return FrameEvent{Frame: decoder.Frame{Data: f}}
}

// TestLoopStatus covers the four states and their per-grid recording dots.
func TestLoopStatus(t *testing.T) {
	cases := []struct {
		name      string
		ft        ecm.FuelTrim
		hasGood   bool
		badge     string
		blm, intg string // "BLM ●" etc.
		o2        string
		suffix    string
	}{
		{"closed+enabled", ecm.FuelTrim{ClosedLoop: true, BLMEnabled: true}, true, "CLOSED LOOP", "BLM ●", "INT ●", "O2 ●", ""},
		{"closed+disabled", ecm.FuelTrim{ClosedLoop: true, BLMEnabled: false}, true, "CLOSED LOOP", "BLM ○", "INT ●", "O2 ●", "(BLM disabled)"},
		{"open", ecm.FuelTrim{ClosedLoop: false}, true, "OPEN LOOP", "BLM ○", "INT ○", "O2 ●", "(grids frozen)"},
		{"no-good-frame", ecm.FuelTrim{ClosedLoop: true, BLMEnabled: true}, false, "LOOP —", "BLM ○", "INT ○", "O2 ○", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LoopStatus(c.ft, c.hasGood)
			for _, want := range []string{c.badge, c.blm, c.intg, c.o2} {
				if !strings.Contains(got, want) {
					t.Errorf("LoopStatus = %q, missing %q", got, want)
				}
			}
			if c.suffix != "" && !strings.Contains(got, c.suffix) {
				t.Errorf("LoopStatus = %q, missing suffix %q", got, c.suffix)
			}
			if c.suffix == "" && strings.Contains(got, "(") {
				t.Errorf("LoopStatus = %q, expected no suffix", got)
			}
			if b := LoopBadge(c.ft, c.hasGood); b != c.badge {
				t.Errorf("LoopBadge = %q, want %q", b, c.badge)
			}
		})
	}
}

// TestINTBodyGating: a closed-loop frame records and highlights the active
// cell; an open-loop frame shows the frozen status.
func TestINTBodyGating(t *testing.T) {
	g := blm.NewDefault()
	g.Add(1600, 40, 130) // one INT sample in the active cell

	closed := INTBody(g, gridFrame(0x82, 130, 0), 4, 130) // MWAF1 bit7+bit1
	if !strings.Contains(closed, "CLOSED LOOP") || !strings.Contains(closed, "INT 130") {
		t.Errorf("closed-loop INTBody missing status:\n%s", closed)
	}
	if !strings.Contains(closed, "\033[7m") {
		t.Error("closed-loop INTBody missing active-cell highlight")
	}

	open := INTBody(g, gridFrame(0x00, 130, 0), 4, 130) // MWAF1=0 → open loop
	if !strings.Contains(open, "integrator frozen") {
		t.Errorf("open-loop INTBody missing frozen status:\n%s", open)
	}
	if strings.Contains(open, "\033[7m") {
		t.Error("open-loop INTBody should not highlight an active cell")
	}
}

// TestO2BodyPrecision: O2 is ungated; the current-reading status keeps full
// 3-decimal precision, but grid cells render to 2 decimals so columns don't
// collide (each cell gets a leading-space gutter).
func TestO2BodyPrecision(t *testing.T) {
	g := blm.NewDefault()
	g.Add(1600, 40, 0.834)

	out := O2Body(g, gridFrame(0x00, 0, 188), 0.834) // open loop, but O2 still shows
	if !strings.Contains(out, "O2 0.834 V") {
		t.Errorf("O2Body status missing 3-decimal voltage:\n%s", out)
	}
	// 3-decimal precision appears only in the status line; the grid cell rounds
	// to 2 decimals (" 0.83").
	if n := strings.Count(out, "0.834"); n != 1 {
		t.Errorf("O2Body has %d occurrences of 0.834, want 1 (status only — cells are 2-decimal):\n%s", n, out)
	}
	if !strings.Contains(out, " 0.83") {
		t.Errorf("O2Body cell should show the 2-decimal average (0.83):\n%s", out)
	}
	if !strings.Contains(out, "\033[7m") {
		t.Error("O2Body should highlight the active cell even in open loop (ungated)")
	}
}

// TestSensorTableExtrema: the 6-column table shows MIN/MAX; nil extrema falls
// back to the 4-column table.
func TestSensorTableExtrema(t *testing.T) {
	frame := []byte{0x04, 0x18, 0x93, 0x75, 0x53, 0x00, 0x5B, 0x43, 0x36, 0x80, 0x69, 0x00, 0x00, 0x00, 0x00, 0x87, 0x80, 0x70, 0x7D, 0xC8}
	registry := ecm.NewRegistry()
	def, _ := registry.GetDefinition("1227747")
	ev := FrameEvent{Frame: decoder.Frame{Data: frame}}

	mins := map[string]float64{"engine_rpm": 550, "battery_voltage": 12.1}
	maxs := map[string]float64{"engine_rpm": 3700, "battery_voltage": 14.6}
	out := SensorTableExtrema(ev, def, mins, maxs)
	for _, want := range []string{"MIN", "MAX", "550", "3700", "12.10", "14.60"} {
		if !strings.Contains(out, want) {
			t.Errorf("SensorTableExtrema missing %q:\n%s", want, out)
		}
	}

	// nil extrema → 4-column fallback (no MIN/MAX headers).
	fallback := SensorTableExtrema(ev, def, nil, nil)
	if strings.Contains(fallback, "MIN") || strings.Contains(fallback, "MAX") {
		t.Errorf("nil extrema should render the 4-column table:\n%s", fallback)
	}
	if fallback != SensorTable(ev, def) {
		t.Error("nil extrema should equal SensorTable output")
	}
}
