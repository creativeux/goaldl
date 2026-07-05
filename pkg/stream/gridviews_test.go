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
		spark     string // ungated like O2: ● whenever a frame parses
		suffix    string
	}{
		{"closed+enabled", ecm.FuelTrim{ClosedLoop: true, BLMEnabled: true}, true, "CLOSED LOOP", "BLM ●", "INT ●", "O2 ●", "SPARK ●", ""},
		{"closed+disabled", ecm.FuelTrim{ClosedLoop: true, BLMEnabled: false}, true, "CLOSED LOOP", "BLM ○", "INT ●", "O2 ●", "SPARK ●", "(BLM disabled)"},
		{"open", ecm.FuelTrim{ClosedLoop: false}, true, "OPEN LOOP", "BLM ○", "INT ○", "O2 ●", "SPARK ●", "(grids frozen)"},
		{"no-good-frame", ecm.FuelTrim{ClosedLoop: true, BLMEnabled: true}, false, "LOOP —", "BLM ○", "INT ○", "O2 ○", "SPARK ○", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LoopStatus(c.ft, c.hasGood)
			for _, want := range []string{c.badge, c.blm, c.intg, c.o2, c.spark} {
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

	closed := INTBody(g, gridFrame(0x82, 130, 0), 4, 130, false) // MWAF1 bit7+bit1
	if !strings.Contains(closed, "CLOSED LOOP") || !strings.Contains(closed, "INT 130") {
		t.Errorf("closed-loop INTBody missing status:\n%s", closed)
	}
	if !strings.Contains(closed, "\033[7m") {
		t.Error("closed-loop INTBody missing active-cell highlight")
	}

	open := INTBody(g, gridFrame(0x00, 130, 0), 4, 130, false) // MWAF1=0 → open loop
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

	out := O2Body(g, gridFrame(0x00, 0, 188), 0.834, false) // open loop, but O2 still shows
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

// TestSparkBody: spark cells show the grid's Sum (total knocks), not the mean
// delta; the view is ungated (active-cell highlight and no dimming even in
// open loop) and the status carries the raw counter.
func TestSparkBody(t *testing.T) {
	g := blm.NewSpark()
	g.Add(1600, 40, 2) // two knock events in one cell: deltas 2 + 3
	g.Add(1600, 40, 3)

	out := SparkBody(g, gridFrame(0x00, 0, 0), 112, false, true) // open loop; showInfo → explainer landmark
	if !strings.Contains(out, "KNOCK_CNT 112") {
		t.Errorf("SparkBody status missing raw counter:\n%s", out)
	}
	if !strings.Contains(out, "    5") {
		t.Errorf("SparkBody cell should show the sum 5 (deltas 2+3), not the mean:\n%s", out)
	}
	if !strings.Contains(out, "\033[7m") {
		t.Error("SparkBody should highlight the active cell (ungated, like O2)")
	}
	// Grid cells never dim (minCount is 1) — check the grid portion only; the
	// explainer block below it is deliberately dim-rendered (cut at its
	// dim-prefixed first line so the escape stays out of the grid portion).
	grid, explainer, found := strings.Cut(out, ansiDim+"  SPARK — knock events")
	if !found {
		t.Fatalf("SparkBody missing the explainer block:\n%s", out)
	}
	if strings.Contains(grid, ansiDim) {
		t.Error("SparkBody cells should never dim (minCount is 1)")
	}
	if !strings.Contains(explainer, "detonation") {
		t.Errorf("spark explainer should say what a knock count means:\n%s", explainer)
	}
	// WinALDL spark axes, not the trim axes: MAP columns start at 30, step 5.
	if !strings.Contains(out, "  30   35") {
		t.Errorf("SparkBody header should show the 30/35 MAP columns:\n%s", out)
	}
}

// TestSparkBodyFreeRunning: with freeRunning set, the status line carries the
// warning and the explainer swaps to its free-run head; with it clear, the
// output is byte-identical to the normal SparkBody (guards the normal path
// against accidental drift).
func TestSparkBodyFreeRunning(t *testing.T) {
	g := blm.NewSpark()
	g.Add(1600, 40, 2)
	ev := gridFrame(0x00, 0, 0)

	warned := SparkBody(g, ev, 112, true, true)
	if !strings.Contains(warned, "free-running counter — not knock") {
		t.Errorf("free-running SparkBody should warn in the status line:\n%s", warned)
	}
	if !strings.Contains(warned, "KNOCK_CNT is free-running") {
		t.Errorf("free-running SparkBody should swap the explainer head:\n%s", warned)
	}
	if strings.Contains(warned, "The goal is 0") {
		t.Errorf("free-running explainer must not keep the 'goal is 0' line:\n%s", warned)
	}
	// Grid values unchanged: the cell sum still shows at full brightness.
	if !strings.Contains(warned, "    2") {
		t.Errorf("free-running SparkBody must still show the grid values:\n%s", warned)
	}

	normal := SparkBody(g, ev, 112, false, true)
	if strings.Contains(normal, "free-running") {
		t.Errorf("normal SparkBody must not warn:\n%s", normal)
	}
	if !strings.Contains(normal, "The goal is 0") {
		t.Errorf("normal SparkBody keeps its usual explainer:\n%s", normal)
	}
}

// TestGridExplainers: each grid view carries its always-visible "what this
// table means" block; the streaming BLMBody (monitor -blm) keeps the compact
// one-line legend instead.
func TestGridExplainers(t *testing.T) {
	g := blm.NewDefault()
	ev := gridFrame(0x82, 130, 188)

	if out := BLMBodyExplained(g, ev, 4); !strings.Contains(out, "Block Learn Multiplier") || !strings.Contains(out, "avg/128") {
		t.Errorf("BLMBodyExplained missing the meaning/act lines:\n%s", out)
	}
	if out := BLMBody(g, ev, 4); strings.Contains(out, "Block Learn Multiplier") || !strings.Contains(out, "target 128") {
		t.Errorf("BLMBody (monitor) should keep the compact legend, not the explainer:\n%s", out)
	}
	if out := INTBody(g, ev, 4, 130, true); !strings.Contains(out, "Integrator") || !strings.Contains(out, "learned into BLM") {
		t.Errorf("INTBody (showInfo) missing its explainer:\n%s", out)
	}
	if out := O2Body(g, ev, 0.834, true); !strings.Contains(out, "stoichiometric") || !strings.Contains(out, "0.45") {
		t.Errorf("O2Body (showInfo) missing its explainer:\n%s", out)
	}
}

// TestGridLegendAccordion: with showInfo false (the dashboard default) the grid
// tabs render the compact one-line legend, not the multi-line explainer.
func TestGridLegendAccordion(t *testing.T) {
	g := blm.NewDefault()
	ev := gridFrame(0x82, 130, 188)

	if out := INTBody(g, ev, 4, 130, false); strings.Contains(out, "learned into BLM") || !strings.Contains(out, "read sustained cell averages") {
		t.Errorf("collapsed INTBody should show the compact legend, not the explainer:\n%s", out)
	}
	if out := O2Body(g, ev, 0.834, false); strings.Contains(out, "stoichiometric") || !strings.Contains(out, "oscillates in closed loop") {
		t.Errorf("collapsed O2Body should show the compact legend, not the explainer:\n%s", out)
	}
	if out := SparkBody(g, ev, 9, false, false); strings.Contains(out, "false knock") || !strings.Contains(out, "goal is 0 everywhere") {
		t.Errorf("collapsed SparkBody should show the compact legend, not the explainer:\n%s", out)
	}
	// A collapsed, free-running Spark tab still carries the warning (in both the
	// status line and the compact legend), even without the full explainer.
	if out := SparkBody(g, ev, 9, true, false); !strings.Contains(out, "free-running") || strings.Contains(out, "working ESC") {
		t.Errorf("collapsed free-running SparkBody should warn without the explainer:\n%s", out)
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
