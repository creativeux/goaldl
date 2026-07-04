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
	recorded  int
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
	ft := ecm.FuelTrimSample(ev.Frame.Data)
	ar, ac := -1, -1
	if ft.Recordable() {
		v.Grid.Add(ft.RPM, ft.MapKPa, ft.BLM)
		ar, ac = v.Grid.Cell(ft.RPM, ft.MapKPa)
		v.recorded++
	}
	if !v.isTTY {
		return // live grid is only meaningful on a terminal
	}
	body := v.format(ev, ft, ar, ac)
	if v.lastLines > 0 {
		fmt.Fprintf(v.w, "\033[%dA", v.lastLines)
	}
	for line := range strings.SplitSeq(body, "\n") {
		fmt.Fprintf(v.w, "\033[2K%s\n", line)
	}
	v.lastLines = strings.Count(body, "\n") + 1
}

// format renders the Wide Average grid with the active cell highlighted.
func (v *BLMView) format(ev FrameEvent, ft ecm.FuelTrim, ar, ac int) string {
	avg := v.Grid.Average()
	samples := v.Grid.Samples()

	var status string
	switch {
	case ft.ClosedLoop && ft.BLMEnabled:
		prog := ""
		if ar >= 0 {
			prog = fmt.Sprintf("  cell %.0f/%d", math.Min(float64(samples[ar][ac]), float64(v.minCount)), v.minCount)
		}
		status = fmt.Sprintf("CLOSED LOOP  RPM %.0f  MAP %.0f kPa  BLM %.0f%s", ft.RPM, ft.MapKPa, ft.BLM, prog)
	case !ft.ClosedLoop:
		status = "OPEN LOOP — not recording (WOT / decel / cold)"
	default:
		status = "block learn disabled — not recording"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s   frame %d   t=%.1fs   %d cells ready   %s\n",
		v.title, ev.Index, ev.Elapsed.Seconds(), v.Grid.PopulatedCells(v.minCount), status)
	// Column header.
	b.WriteString("  RPM\\MAP")
	for _, m := range v.Grid.MAP {
		fmt.Fprintf(&b, " %4.0f", m)
	}
	b.WriteByte('\n')
	for r, rpm := range v.Grid.RPM {
		fmt.Fprintf(&b, "  %5.0f ", rpm)
		for c := range v.Grid.MAP {
			var cellText string
			switch {
			case samples[r][c] == 0:
				cellText = "    ·"
			case samples[r][c] < v.minCount:
				// Still accumulating: dim so it reads as provisional.
				cellText = fmt.Sprintf("\033[2m%5.0f\033[0m", math.Round(avg[r][c]))
			default:
				cellText = fmt.Sprintf("%5.0f", math.Round(avg[r][c]))
			}
			if r == ar && c == ac {
				cellText = "\033[7m" + strings.TrimPrefix(strings.TrimSuffix(cellText, "\033[0m"), "\033[2m") + "\033[0m"
			}
			b.WriteString(cellText)
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "  target 128:  >128 lean, <128 rich;  · = no data, dim = <%d samples", v.minCount)
	return b.String()
}
