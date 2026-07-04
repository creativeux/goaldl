package stream

import (
	"fmt"
	"math"
	"strings"

	"goaldl/pkg/blm"
	"goaldl/pkg/ecm"
)

// Grid-tab view builders (BLM, INT, O2) and the persistent loop-state line.
// All are pure content builders in the BLMBody idiom — terminal strings with
// inline ANSI emphasis, no positioning codes — so the TUI (and a future serve
// adapter) render the same accumulator state uniformly. INT and O2 are the same
// blm.Grid on the default axes with different value sources and gating (INT
// closed-loop gated, O2 ungated 3-decimal volts); see spec-phase2.md §1-2.

// gridHeat renders a status line, the RPM×MAP heatmap (active cell at (ar,ac)
// reverse-highlighted, cells below minCount dimmed as still-accumulating, "·"
// for empty), and a legend. prec is the decimal precision per cell (0 for
// BLM/INT counts, 3 for O2 volts). It reads the grid but never modifies it.
func gridHeat(g *blm.Grid, ar, ac, minCount, prec int, status, legend string) string {
	avg := g.Average()
	samples := g.Samples()

	var b strings.Builder
	fmt.Fprintln(&b, status)
	b.WriteString("  RPM\\MAP")
	for _, m := range g.MAP {
		fmt.Fprintf(&b, " %4.0f", m)
	}
	b.WriteByte('\n')
	for r, rpm := range g.RPM {
		fmt.Fprintf(&b, "  %5.0f ", rpm)
		for c := range g.MAP {
			var cellText string
			switch {
			case samples[r][c] == 0:
				cellText = "    ·"
			case samples[r][c] < minCount:
				cellText = ansiDim + fmtCell(avg[r][c], prec) + ansiReset
			default:
				cellText = fmtCell(avg[r][c], prec)
			}
			if r == ar && c == ac {
				cellText = "\033[7m" + strings.TrimPrefix(strings.TrimSuffix(cellText, ansiReset), ansiDim) + ansiReset
			}
			b.WriteString(cellText)
		}
		b.WriteByte('\n')
	}
	b.WriteString(legend)
	return b.String()
}

// fmtCell formats one grid cell to a fixed 5-column width. prec 0 rounds
// half-away-from-zero (via math.Round) to preserve the historical BLM display;
// prec > 0 formats directly (e.g. O2 volts to 3 decimals).
func fmtCell(v float64, prec int) string {
	if prec == 0 {
		return fmt.Sprintf("%5.0f", math.Round(v))
	}
	return fmt.Sprintf("%5.*f", prec, v)
}

// INTBody renders the integrator (short-term fuel trim) grid. Like BLM it bins
// by RPM×MAP and shows the Wide Average, but it gates on closed loop only
// (block-learn-enable is a BLM-specific gate). intVal is the current frame's
// integrator, shown live in the status line.
func INTBody(g *blm.Grid, ev FrameEvent, minCount int, intVal float64) string {
	ft := ecm.FuelTrimSample(ev.Frame.Data)
	ar, ac := -1, -1
	status := "OPEN LOOP — integrator frozen"
	if ft.ClosedLoop {
		ar, ac = g.Cell(ft.RPM, ft.MapKPa)
		prog := fmt.Sprintf("  cell %.0f/%d",
			math.Min(float64(g.Samples()[ar][ac]), float64(minCount)), minCount)
		status = fmt.Sprintf("CLOSED LOOP  RPM %.0f  MAP %.0f kPa  INT %.0f%s",
			ft.RPM, ft.MapKPa, intVal, prog)
	}
	return gridHeat(g, ar, ac, minCount, 0, status,
		"  target 128:  >128 adding fuel (lean), <128 removing (rich)")
}

// O2Body renders the oxygen-sensor voltage grid. O2 is ungated (populates every
// parsed frame); minCount is 1 so cells appear solid as soon as they have a
// sample. The grid cells render to 2 decimals so the dense heatmap keeps a
// leading-space gutter between columns (a 3-decimal cell fills the whole 5-wide
// column and columns collide); the current reading and the saved file keep full
// 3-decimal precision. o2Volts is the current frame's O2 voltage.
func O2Body(g *blm.Grid, ev FrameEvent, o2Volts float64) string {
	ft := ecm.FuelTrimSample(ev.Frame.Data)
	ar, ac := g.Cell(ft.RPM, ft.MapKPa)
	status := fmt.Sprintf("O2 %.3f V  RPM %.0f  MAP %.0f kPa", o2Volts, ft.RPM, ft.MapKPa)
	return gridHeat(g, ar, ac, 1, 2, status, "  volts; higher = richer exhaust; · = no data")
}

// LoopBadge is the loop-state word: CLOSED LOOP / OPEN LOOP, or "LOOP —" before
// the first parseable frame. The TUI colors this token; the word is stable so
// the caller can strip it off LoopStatus to color it in place.
func LoopBadge(ft ecm.FuelTrim, hasGood bool) string {
	switch {
	case !hasGood:
		return "LOOP —"
	case ft.ClosedLoop:
		return "CLOSED LOOP"
	default:
		return "OPEN LOOP"
	}
}

// LoopStatus renders the persistent, plain-text loop/recording line shown under
// the tab bar on every tab: the loop badge plus per-grid ● accumulating / ○
// frozen markers, so the operator can tell from any tab whether the grid they
// are on is live. O2 accumulates whenever a frame parses (ungated); BLM needs
// closed loop + block-learn enabled; INT needs closed loop.
func LoopStatus(ft ecm.FuelTrim, hasGood bool) string {
	dot := func(on bool) string {
		if on {
			return "●"
		}
		return "○"
	}
	blmOn := hasGood && ft.ClosedLoop && ft.BLMEnabled
	intOn := hasGood && ft.ClosedLoop
	o2On := hasGood

	suffix := ""
	switch {
	case hasGood && !ft.ClosedLoop:
		suffix = "  (grids frozen)"
	case hasGood && !ft.BLMEnabled:
		suffix = "  (BLM disabled)"
	}
	return fmt.Sprintf("%s   rec: BLM %s INT %s O2 %s%s",
		LoopBadge(ft, hasGood), dot(blmOn), dot(intOn), dot(o2On), suffix)
}
