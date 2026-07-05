package stream

import (
	"fmt"
	"io"
	"math"
	"strings"

	"goaldl/pkg/blm"
	"goaldl/pkg/ecm"
)

// BLMView accumulates fuel-trim samples from a frame stream into a BLM grid and
// (on a TTY) redraws it live, highlighting the cell the engine is currently in.
// Off a TTY it just accumulates; the caller prints the final grid. The Grid is
// exported so the caller can render final Average/Correction tables after the
// stream ends.
type BLMView struct {
	w         io.Writer
	isTTY     bool
	Grid      *blm.Grid
	title     string
	minCount  int // samples before a cell is trusted (shown solid vs dim)
	lastLines int
}

// NewBLMView builds a live BLM view over a fresh default grid. A cell is drawn
// dim (still accumulating) until it reaches minSamples, then solid.
func NewBLMView(w io.Writer, isTTY bool, title string, minSamples int) *BLMView {
	if minSamples < 1 {
		minSamples = 1
	}
	return &BLMView{w: w, isTTY: isTTY, Grid: blm.NewDefault(), title: title, minCount: minSamples}
}

// Render observes one frame: records it if valid, then redraws the grid live.
func (v *BLMView) Render(ev FrameEvent) {
	if ft := ecm.FuelTrimSample(ev.Frame.Data); ft.Recordable() {
		v.Grid.Add(ft.RPM, ft.MapKPa, ft.BLM)
	}
	if !v.isTTY {
		return // live grid is only meaningful on a terminal
	}
	titleLine := fmt.Sprintf("%s   frame %d   t=%.1fs   %d cells ready",
		v.title, ev.Index, ev.Elapsed.Seconds(), v.Grid.PopulatedCells(v.minCount))
	// width 0: the streaming view is a fixed in-place redraw, not width-clamped.
	body := titleLine + "\n" + BLMBody(v.Grid, ev, v.minCount, 0)
	if v.lastLines > 0 {
		fmt.Fprintf(v.w, "\033[%dA", v.lastLines)
	}
	for line := range strings.SplitSeq(body, "\n") {
		fmt.Fprintf(v.w, "\033[2K%s\n", line)
	}
	v.lastLines = strings.Count(body, "\n") + 1
}

// blmCompactLegend is the one-line BLM legend (target/lean/rich + dim key).
func blmCompactLegend(minCount int) string {
	return fmt.Sprintf("  target 128:  >128 lean, <128 rich;  · = no data, dim = <%d samples", minCount)
}

// BLMBody renders the BLM status line, the Wide-Average grid (active cell
// reverse-highlighted, cells below minCount dimmed as still-accumulating), and
// the compact one-line legend — the streaming `monitor -blm` variant, which
// redraws in place and keeps its chrome tight (it has no bottom bar, so it keeps
// the live status line). width caps the MAP columns (0 = no limit; monitor 0).
func BLMBody(g *blm.Grid, ev FrameEvent, minCount, width int) string {
	return blmBody(g, ev, minCount, blmCompactLegend(minCount), width, true)
}

// BLMBodyDash is the dashboard BLM tab: no status line (the bottom bar shows
// loop state), and the compact legend or the full explainer per showInfo.
func BLMBodyDash(g *blm.Grid, ev FrameEvent, minCount, width int, showInfo bool) string {
	legend := blmCompactLegend(minCount)
	if showInfo {
		legend = blmExplainer
	}
	return blmBody(g, ev, minCount, legend, width, false)
}

// showStatus renders the live CLOSED/OPEN-LOOP reading above the grid (the
// streaming monitor); the dashboard passes false so the grid gets that row back.
func blmBody(g *blm.Grid, ev FrameEvent, minCount int, legend string, width int, showStatus bool) string {
	ft := ecm.FuelTrimSample(ev.Frame.Data)
	ar, ac := -1, -1
	if ft.Recordable() {
		ar, ac = g.Cell(ft.RPM, ft.MapKPa)
	}
	status := ""
	if showStatus {
		switch {
		case ft.ClosedLoop && ft.BLMEnabled:
			prog := ""
			if ar >= 0 {
				prog = fmt.Sprintf("  cell %.0f/%d", math.Min(float64(g.Samples()[ar][ac]), float64(minCount)), minCount)
			}
			status = fmt.Sprintf("CLOSED LOOP  RPM %.0f  MAP %.0f kPa  BLM %.0f%s", ft.RPM, ft.MapKPa, ft.BLM, prog)
		case !ft.ClosedLoop:
			status = "OPEN LOOP — not recording (WOT / decel / cold)"
		default:
			status = "block learn disabled — not recording"
		}
	}
	return gridHeat(g, g.Average(), ar, ac, minCount, 0, status, legend, width)
}
