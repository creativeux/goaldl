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
	recorded  int
	lastLines int
}

// NewBLMView builds a live BLM view over a fresh default grid.
func NewBLMView(w io.Writer, isTTY bool, title string) *BLMView {
	return &BLMView{w: w, isTTY: isTTY, Grid: blm.NewDefault(), title: title}
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
	body := v.format(ft, ar, ac)
	if v.lastLines > 0 {
		fmt.Fprintf(v.w, "\033[%dA", v.lastLines)
	}
	for _, line := range strings.Split(body, "\n") {
		fmt.Fprintf(v.w, "\033[2K%s\n", line)
	}
	v.lastLines = strings.Count(body, "\n") + 1
}

// format renders the Wide Average grid with the active cell highlighted.
func (v *BLMView) format(ft ecm.FuelTrim, ar, ac int) string {
	avg := v.Grid.Average()
	samples := v.Grid.Samples()

	var status string
	switch {
	case ft.ClosedLoop && ft.BLMEnabled:
		status = fmt.Sprintf("CLOSED LOOP  RPM %.0f  MAP %.0f kPa  BLM %.0f", ft.RPM, ft.MapKPa, ft.BLM)
	case !ft.ClosedLoop:
		status = "OPEN LOOP — not recording (WOT / decel / cold)"
	default:
		status = "block learn disabled — not recording"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s   recorded %d   %s\n", v.title, v.recorded, status)
	// Column header.
	b.WriteString("  RPM\\MAP")
	for _, m := range v.Grid.MAP {
		fmt.Fprintf(&b, " %4.0f", m)
	}
	b.WriteByte('\n')
	for r, rpm := range v.Grid.RPM {
		fmt.Fprintf(&b, "  %5.0f ", rpm)
		for c := range v.Grid.MAP {
			cellText := "    ·"
			if samples[r][c] > 0 {
				cellText = fmt.Sprintf("%5.0f", math.Round(avg[r][c]))
			}
			if r == ar && c == ac {
				cellText = "\033[7m" + cellText + "\033[0m" // reverse-video the active cell
			}
			b.WriteString(cellText)
		}
		b.WriteByte('\n')
	}
	b.WriteString("  target 128:  >128 lean, <128 rich;  · = no data")
	return b.String()
}
