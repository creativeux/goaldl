package stream

import (
	"fmt"
	"io"
	"math"
	"strings"

	"goaldl/pkg/aldl"
	"goaldl/pkg/ecm"
)

// Row is one line of the sensor table: the human-readable sensor name, its raw
// bytes from the frame, and the translated value with unit.
type Row struct {
	Sensor string
	Raw    string
	Value  string
}

// BuildRows turns a frame and its parsed values into table rows, one per ECM
// parameter in definition order. Raw comes straight from the frame bytes;
// Value comes from parsed (missing entries render as "—"). This is pure so it
// can be unit-tested without a terminal.
func BuildRows(frame []byte, def *ecm.Definition, parsed map[string]float64) []Row {
	rows := make([]Row, 0, len(def.Parameters))
	for _, p := range def.Parameters {
		rows = append(rows, Row{
			Sensor: sensorLabel(p),
			Raw:    formatRaw(frame, p),
			Value:  formatValue(parsed, p),
		})
	}
	return rows
}

func sensorLabel(p ecm.Parameter) string {
	if p.Description != "" {
		return p.Description
	}
	return p.Name
}

// formatRaw renders the parameter's bytes as hex; a single byte also shows its
// decimal value, since that's what most sensor formulas act on.
func formatRaw(frame []byte, p ecm.Parameter) string {
	if p.Offset < 0 || p.Offset+p.Size > len(frame) {
		return "—"
	}
	b := frame[p.Offset : p.Offset+p.Size]
	if len(b) == 1 {
		return fmt.Sprintf("0x%02X (%d)", b[0], b[0])
	}
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = fmt.Sprintf("0x%02X", x)
	}
	return strings.Join(parts, " ")
}

// formatValue renders the translated value, dropping a trailing ".00" for
// whole numbers and appending the unit when present.
func formatValue(parsed map[string]float64, p ecm.Parameter) string {
	v, ok := parsed[p.Name]
	if !ok {
		return "—"
	}
	var s string
	if math.Abs(v-math.Round(v)) < 1e-9 {
		s = fmt.Sprintf("%.0f", v)
	} else {
		s = fmt.Sprintf("%.2f", v)
	}
	if p.Unit != "" {
		s += " " + p.Unit
	}
	return s
}

// renderTable formats rows into an aligned three-column table (no trailing
// newline on the last line).
func renderTable(rows []Row) string {
	const (
		hSensor = "SENSOR"
		hRaw    = "RAW"
		hValue  = "VALUE"
	)
	wS, wR := len(hSensor), len(hRaw)
	for _, r := range rows {
		wS = max(wS, len(r.Sensor))
		wR = max(wR, len(r.Raw))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s  %-*s  %s\n", wS, hSensor, wR, hRaw, hValue)
	fmt.Fprintf(&b, "%s\n", strings.Repeat("─", wS+wR+len(hValue)+4))
	for i, r := range rows {
		fmt.Fprintf(&b, "%-*s  %-*s  %s", wS, r.Sensor, wR, r.Raw, r.Value)
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Renderer draws the live sensor table, redrawing in place on a TTY.
type Renderer struct {
	w         io.Writer
	isTTY     bool
	registry  *ecm.Registry
	ecmPart   string
	promID    int
	title     string
	lastLines int
}

// NewRenderer builds a table renderer. promID is the expected PROM ID for the
// header's sync indicator (0 disables it).
func NewRenderer(w io.Writer, isTTY bool, registry *ecm.Registry, ecmPart string, promID int, title string) *Renderer {
	return &Renderer{w: w, isTTY: isTTY, registry: registry, ecmPart: ecmPart, promID: promID, title: title}
}

// Render draws the table for one frame event. On a TTY it moves the cursor
// back up over the previous table and overwrites it; otherwise it prints each
// frame as a fresh block.
func (r *Renderer) Render(ev FrameEvent) {
	data, err := r.registry.ParseFrame(&aldl.Frame{Data: ev.Frame.Data}, r.ecmPart)
	var rows []Row
	def, _ := r.registry.GetDefinition(r.ecmPart)
	if err == nil {
		rows = BuildRows(ev.Frame.Data, def, data.ParsedValues)
	} else {
		rows = BuildRows(ev.Frame.Data, def, map[string]float64{})
	}

	header := fmt.Sprintf("%s   frame %d   t=%.1fs   %s",
		r.title, ev.Index, ev.Elapsed.Seconds(), r.promMark(ev.Frame.Data))
	body := header + "\n" + renderTable(rows)

	if r.isTTY {
		if r.lastLines > 0 {
			fmt.Fprintf(r.w, "\033[%dA", r.lastLines) // cursor up over previous render
		}
		// Clear each line as we rewrite so shorter content can't leave artifacts.
		for _, line := range strings.Split(body, "\n") {
			fmt.Fprintf(r.w, "\033[2K%s\n", line)
		}
		r.lastLines = strings.Count(body, "\n") + 1
		return
	}
	// Non-interactive (piped/redirected): plain blocks, no ANSI.
	fmt.Fprintln(r.w, body)
	fmt.Fprintln(r.w)
}

func (r *Renderer) promMark(frame []byte) string {
	if r.promID == 0 || len(frame) < 3 {
		return ""
	}
	if int(frame[1])<<8|int(frame[2]) == r.promID {
		return "PROM ✓"
	}
	return "PROM ✗"
}
