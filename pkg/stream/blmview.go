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
	body := titleLine + "\n" + BLMBody(v.Grid, ev, v.minCount)
	if v.lastLines > 0 {
		fmt.Fprintf(v.w, "\033[%dA", v.lastLines)
	}
	for line := range strings.SplitSeq(body, "\n") {
		fmt.Fprintf(v.w, "\033[2K%s\n", line)
	}
	v.lastLines = strings.Count(body, "\n") + 1
}

// BLMBody renders the BLM status line, the Wide-Average grid (active cell
// reverse-highlighted, cells below minCount dimmed as still-accumulating), and
// the legend — as a string with no title/frame/time chrome, for embedding in a
// TUI or the streaming view. It reads the grid but does not modify it.
func BLMBody(g *blm.Grid, ev FrameEvent, minCount int) string {
	ft := ecm.FuelTrimSample(ev.Frame.Data)
	ar, ac := -1, -1
	if ft.Recordable() {
		ar, ac = g.Cell(ft.RPM, ft.MapKPa)
	}
	var status string
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

	legend := fmt.Sprintf("  target 128:  >128 lean, <128 rich;  · = no data, dim = <%d samples", minCount)
	return gridHeat(g, ar, ac, minCount, 0, status, legend)
}
