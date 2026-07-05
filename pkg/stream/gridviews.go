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
// for empty), and a legend. values is the per-cell number to show — Average()
// for the trim/O2 grids, Sum() for spark knock counts. prec is the decimal
// precision per cell (0 for BLM/INT counts, 2 for O2 volts). width caps the
// number of MAP columns drawn so a wide grid (the Spark axes are 15 columns)
// truncates at a whole-column boundary with a › cue rather than wrapping or
// cutting a number mid-digit (width<=0 = no limit). It reads the grid but never
// modifies it.
func gridHeat(g *blm.Grid, values [][]float64, ar, ac, minCount, prec int, status, legend string, width int) string {
	samples := g.Samples()

	// "  RPM\MAP" label is 9 cells; each MAP column is 5; reserve 2 for " ›".
	// gridHeat self-limits so its lines never exceed width — the caller's
	// ANSI catch-all must never have to cut a cell (that would show a partial,
	// misleading number). When not even one column fits, emit the label + › only.
	const labelW, cellW = 9, 5
	cols := len(g.MAP)
	truncated := false
	if width > 0 {
		switch fit := (width - labelW - 2) / cellW; {
		case fit < 1:
			cols, truncated = 0, true
		case fit < cols:
			cols, truncated = fit, true
		}
	}

	var b strings.Builder
	// The status line is optional: the dashboard suppresses it (the bottom bar
	// already shows loop state), leaving the grid to breathe; the streaming
	// monitor passes a live status. Empty ⇒ no line at all (not a blank).
	if status != "" {
		fmt.Fprintln(&b, status)
	}
	b.WriteString("  RPM\\MAP")
	for c := 0; c < cols; c++ {
		fmt.Fprintf(&b, " %4.0f", g.MAP[c])
	}
	if truncated {
		b.WriteString(" ›")
	}
	b.WriteByte('\n')
	for r, rpm := range g.RPM {
		fmt.Fprintf(&b, "  %5.0f ", rpm)
		for c := 0; c < cols; c++ {
			var cellText string
			switch {
			case samples[r][c] == 0:
				cellText = "    ·"
			case samples[r][c] < minCount:
				cellText = ansiDim + fmtCell(values[r][c], prec) + ansiReset
			default:
				cellText = fmtCell(values[r][c], prec)
			}
			if r == ar && c == ac {
				cellText = "\033[7m" + strings.TrimPrefix(strings.TrimSuffix(cellText, ansiReset), ansiDim) + ansiReset
			}
			b.WriteString(cellText)
		}
		if truncated {
			b.WriteString(" ›")
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

// Grid explainers: the always-visible "what this table means" block rendered
// dim under each grid tab, in place of a terse one-line legend (user request,
// 2026-07-04). Written for the at-the-car operator: what the table is, how to
// read it, and how to act on it. BLM's lives here but is applied only by the
// dashboard (BLMBodyExplained) — `monitor -blm` keeps the compact legend for
// its in-place streaming redraw.
const (
	blmExplainer = ansiDim + `  BLM — Block Learn Multiplier: the fuel correction the ECM has LEARNED
  long-term for each RPM×MAP cell (recorded in closed loop with block learn
  enabled). 128 = base fuel table correct here · >128 ECM is adding fuel →
  your tune is LEAN there · <128 removing fuel → RICH.
  Act: multiply that cell's base VE/fuel by avg/128 — the saved Correction
  table does this math.  · = no data · dim = too few samples to trust yet` + ansiReset

	intExplainer = ansiDim + `  INT — Integrator: the ECM's short-term fuel correction RIGHT NOW (resets
  constantly; its persistent lean/rich error is what gets learned into BLM).
  128 = no correction · >128 adding fuel (lean) · <128 removing (rich).
  It hunts by design in closed loop — read sustained cell averages, not
  single samples. If INT and BLM lean the same way in a cell, the mixture
  error there is real.` + ansiReset

	o2Explainer = ansiDim + `  O2 — narrowband oxygen sensor voltage, averaged per cell. ~0.45 V =
  stoichiometric · below ~0.3 V lean · above ~0.7 V rich. In closed loop it
  oscillates around 0.45 by design (cell averages near 0.45 are healthy);
  a cell stuck high or low is a real mixture offset the trims may be
  masking. Ungated — records every frame, so open-loop cells (WOT / cold)
  show the true uncorrected mixture.` + ansiReset

	// The spark explainer has two heads sharing one tail: the normal one, and a
	// free-running-counter warning shown when the ECM's KNOCK_CNT advances every
	// frame (as on the target vehicle — verified in drive_4800.raw and the
	// WinALDL log). Per the raw-data policy the grid values are still shown at
	// full brightness; only this text and the status line flag that they are a
	// counter artifact, not knock.
	sparkExplainerNormal = ansiDim + `  SPARK — knock events: how many times the ESC counted detonation in each
  cell this session (deltas of the cumulative knock counter). The goal is 0
  everywhere. A repeating count in a cell = too much spark advance or too
  lean under that load — pull timing or add fuel there and re-test.` + sparkExplainerTail

	sparkExplainerFreeRun = ansiDim + `  SPARK — knock events per cell (deltas of the cumulative knock counter).
  ⚠ On THIS vehicle KNOCK_CNT is free-running — it advances every frame, so
  the cell totals below are a counter artifact, NOT a knock count, and are
  not meaningful. On an ECM with a working ESC, a cell total > 0 means
  detonation was counted there.` + sparkExplainerTail

	sparkExplainerTail = `
  A lone count on startup or rough road can be false knock; look for
  repetition before acting.` + ansiReset
)

// Compact one-line legends shown in place of the full explainer when the info
// accordion is collapsed (the dashboard default — `i` toggles it). They keep a
// grid tab readable without the 5–6 line explainer eating vertical space on a
// short terminal. BLM's compact legend lives in BLMBody (shared with the
// streaming `monitor -blm`, which has no `i` key).
const (
	intLegend          = `  target 128 · >128 lean · <128 rich · read sustained cell averages`
	o2Legend           = `  ~0.45 V stoich · <0.3 lean · >0.7 rich · oscillates in closed loop`
	sparkLegend        = `  knock events per cell · goal is 0 everywhere`
	sparkLegendFreeRun = `  ` + ansiBold + `⚠ counter free-running — cell values are not knock` + ansiReset
)

// INTBody renders the integrator (short-term fuel trim) grid. Like BLM it bins
// by RPM×MAP and shows the Wide Average, but it gates on closed loop only
// (block-learn-enable is a BLM-specific gate). intVal is the current frame's
// integrator, shown live in the status line.
// showInfo selects the full explainer (true) or the compact one-line legend
// (false, the dashboard default) — the info accordion toggled by `i`. The grid
// has no status line (the bottom bar carries loop state); the active cell is
// still highlighted in closed loop.
func INTBody(g *blm.Grid, ev FrameEvent, minCount int, showInfo bool, width int) string {
	ft := ecm.FuelTrimSample(ev.Frame.Data)
	ar, ac := -1, -1
	if ft.ClosedLoop {
		ar, ac = g.Cell(ft.RPM, ft.MapKPa)
	}
	legend := intLegend
	if showInfo {
		legend = intExplainer
	}
	return gridHeat(g, g.Average(), ar, ac, minCount, 0, "", legend, width)
}

// O2Body renders the oxygen-sensor voltage grid. O2 is ungated (populates every
// parsed frame); minCount is 1 so cells appear solid as soon as they have a
// sample. The grid cells render to 2 decimals so the dense heatmap keeps a
// leading-space gutter between columns (a 3-decimal cell fills the whole 5-wide
// column and columns collide); the current reading and the saved file keep full
// 3-decimal precision. o2Volts is the current frame's O2 voltage.
func O2Body(g *blm.Grid, ev FrameEvent, showInfo bool, width int) string {
	ft := ecm.FuelTrimSample(ev.Frame.Data)
	ar, ac := g.Cell(ft.RPM, ft.MapKPa)
	legend := o2Legend
	if showInfo {
		legend = o2Explainer
	}
	return gridHeat(g, g.Average(), ar, ac, 1, 2, "", legend, width)
}

// SparkBody renders the knock-events grid: each cell is the total knocks
// counted in that RPM×MAP cell this session (per-frame deltas of the
// cumulative KNOCK_CNT byte, binned by the consumer). Ungated like O2 —
// minCount is 1, cells never dim. Note the cells show the grid's Sum, not its
// Average: a cell fed deltas 2 and 3 reads 5 knocks. knockCnt is the current
// frame's raw counter, shown in the status line. (A mid-session counter reset
// — ECM power cycle — appears as one spurious wrapped delta; accepted, same
// failure mode as WinALDL.)
// freeRunning is the consumer's verdict that KNOCK_CNT is advancing every frame
// (a counter artifact, not knock). When set, the status line gains a warning and
// the legend/explainer notes it; the grid values are unchanged (shown at full
// brightness — raw-data policy: annotate, never hide or dim). showInfo picks the
// full explainer (true) over the compact legend (false, the dashboard default).
func SparkBody(g *blm.Grid, ev FrameEvent, freeRunning, showInfo bool, width int) string {
	ft := ecm.FuelTrimSample(ev.Frame.Data)
	ar, ac := g.Cell(ft.RPM, ft.MapKPa)
	var legend string
	switch {
	case freeRunning && showInfo:
		legend = sparkExplainerFreeRun
	case freeRunning:
		legend = sparkLegendFreeRun
	case showInfo:
		legend = sparkExplainerNormal
	default:
		legend = sparkLegend
	}
	// No status line except the free-running warning — the one piece worth
	// keeping prominent (the raw KNOCK_CNT/RPM/MAP readout is dropped as the
	// bottom bar covers loop state and the values here are not knock anyway).
	status := ""
	if freeRunning {
		status = ansiBold + "⚠ free-running counter — not knock" + ansiReset
	}
	return gridHeat(g, g.Sum(), ar, ac, 1, 0, status, legend, width)
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
// are on is live. O2 and SPARK accumulate whenever a frame parses (ungated);
// BLM needs closed loop + block-learn enabled; INT needs closed loop.
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
	return fmt.Sprintf("%s   rec: BLM %s INT %s O2 %s SPARK %s%s",
		LoopBadge(ft, hasGood), dot(blmOn), dot(intOn), dot(o2On), dot(o2On), suffix)
}
