package stream

import (
	"fmt"
	"io"
	"math"
	"strings"

	"goaldl/pkg/ecm"
)

// Row is one line of the sensor table: the human-readable sensor name, its raw
// bytes from the frame, the translated value with unit, the per-sensor Min/Max
// extrema (blank in the 4-column layout), and the alternate (dual-unit) value
// when the parameter defines one.
type Row struct {
	Sensor string
	Raw    string
	Value  string
	Min    string
	Max    string
	Alt    string
}

// BuildRows turns a frame and its parsed values into table rows, one per ECM
// parameter in definition order. Raw comes straight from the frame bytes;
// Value comes from parsed (missing entries render as "—"); Alt is the
// parameter's alternate conversion applied to the same raw byte (blank when
// none is defined). This is pure so it can be unit-tested without a terminal.
func BuildRows(frame []byte, def *ecm.Definition, parsed map[string]float64) []Row {
	rows := make([]Row, 0, len(def.Parameters))
	for _, p := range def.Parameters {
		rows = append(rows, Row{
			Sensor: sensorLabel(p),
			Raw:    formatRaw(frame, p),
			Value:  formatValue(parsed, p),
			Alt:    formatAlt(frame, p),
		})
	}
	return rows
}

// BuildRowsExtrema is BuildRows plus per-sensor Min/Max columns, pulled from
// the mins/maxs maps (keyed by parameter Name, primary unit). A parameter
// absent from the maps renders "—" (no data since the last reset).
func BuildRowsExtrema(frame []byte, def *ecm.Definition, parsed, mins, maxs map[string]float64) []Row {
	rows := make([]Row, 0, len(def.Parameters))
	for _, p := range def.Parameters {
		rows = append(rows, Row{
			Sensor: sensorLabel(p),
			Raw:    formatRaw(frame, p),
			Value:  formatValue(parsed, p),
			Min:    formatExtreme(mins, p),
			Max:    formatExtreme(maxs, p),
			Alt:    formatAlt(frame, p),
		})
	}
	return rows
}

// formatExtreme renders one Min/Max cell in the parameter's primary unit, or
// "—" when no reading has been recorded for it yet.
func formatExtreme(m map[string]float64, p ecm.Parameter) string {
	v, ok := m[p.Name]
	if !ok {
		return "—"
	}
	return formatNum(v, p.Unit)
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
	return formatNum(v, p.Unit)
}

// formatAlt renders the parameter's alternate (dual-unit) conversion of the
// same raw byte, or blank when the parameter has none. Alternate conversions
// apply to single-byte parameters only.
func formatAlt(frame []byte, p ecm.Parameter) string {
	if p.Alt == nil || p.Size != 1 || p.Offset < 0 || p.Offset >= len(frame) {
		return ""
	}
	raw := frame[p.Offset]
	var v float64
	if p.Alt.Lookup != nil {
		v = p.Alt.Lookup(raw)
	} else {
		v = float64(raw)*p.Alt.Factor + p.Alt.Bias
	}
	return formatNum(v, p.Alt.Unit)
}

func formatNum(v float64, unit string) string {
	var s string
	if math.Abs(v-math.Round(v)) < 1e-9 {
		s = fmt.Sprintf("%.0f", v)
	} else {
		s = fmt.Sprintf("%.2f", v)
	}
	if unit != "" {
		s += " " + unit
	}
	return s
}

// renderTable formats rows into an aligned four-column table (no trailing
// newline on the last line).
func renderTable(rows []Row) string {
	const (
		hSensor = "SENSOR"
		hRaw    = "RAW"
		hValue  = "VALUE"
		hAlt    = "ALT"
	)
	wS, wR, wV := len(hSensor), len(hRaw), len(hValue)
	for _, r := range rows {
		wS = max(wS, len(r.Sensor))
		wR = max(wR, len(r.Raw))
		wV = max(wV, len(r.Value))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %s\n", wS, hSensor, wR, hRaw, wV, hValue, hAlt)
	fmt.Fprintf(&b, "%s\n", strings.Repeat("─", wS+wR+wV+len(hAlt)+6))
	for i, r := range rows {
		fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %s", wS, r.Sensor, wR, r.Raw, wV, r.Value, r.Alt)
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
	def       *ecm.Definition
	promID    int
	title     string
	lastLines int
}

// NewRenderer builds a table renderer over an ECM definition (possibly
// calibrated — see Definition.WithTPSCalibration). promID is the expected PROM
// ID for the header's sync indicator (0 disables it).
func NewRenderer(w io.Writer, isTTY bool, def *ecm.Definition, promID int, title string) *Renderer {
	return &Renderer{w: w, isTTY: isTTY, def: def, promID: promID, title: title}
}

// rowsFor parses a frame and builds its sensor rows, leaving values blank on a
// parse error. Shared by the streaming Renderer and SensorTable.
func rowsFor(ev FrameEvent, def *ecm.Definition) []Row {
	parsed, err := def.Parse(ev.Frame.Data)
	if err != nil {
		parsed = map[string]float64{}
	}
	return BuildRows(ev.Frame.Data, def, parsed)
}

// SensorTable renders one frame's sensor/raw/value/alt table as a string, with
// no terminal control codes — for embedding in a TUI or other presenter.
func SensorTable(ev FrameEvent, def *ecm.Definition) string {
	return renderTable(rowsFor(ev, def))
}

// SensorTableExtrema renders the dashboard table (SENSOR·RAW·VALUE·MIN·MAX·ALT),
// with per-sensor extrema from mins/maxs. When mins is nil it falls back to the
// 4-column SensorTable, so the monitor path is unchanged. width, when > 0, drops
// the lowest-value columns (ALT, then RAW) so the core SENSOR/VALUE/MIN/MAX stay
// readable on a narrow terminal instead of wrapping.
func SensorTableExtrema(ev FrameEvent, def *ecm.Definition, mins, maxs map[string]float64, width int) string {
	if mins == nil {
		return SensorTable(ev, def)
	}
	parsed, err := def.Parse(ev.Frame.Data)
	if err != nil {
		parsed = map[string]float64{}
	}
	return renderTableExtrema(BuildRowsExtrema(ev.Frame.Data, def, parsed, mins, maxs), width)
}

// renderTableExtrema formats rows into the aligned dashboard table (no trailing
// newline on the last line). Columns are dropped in order — ALT first, then RAW
// — while the table is wider than width (width<=0 keeps all columns).
func renderTableExtrema(rows []Row, width int) string {
	cols := []struct {
		header string
		get    func(Row) string
		w      int
	}{
		{"SENSOR", func(r Row) string { return r.Sensor }, 0},
		{"RAW", func(r Row) string { return r.Raw }, 0},
		{"VALUE", func(r Row) string { return r.Value }, 0},
		{"MIN", func(r Row) string { return r.Min }, 0},
		{"MAX", func(r Row) string { return r.Max }, 0},
		{"ALT", func(r Row) string { return r.Alt }, 0},
	}
	for i := range cols {
		w := len(cols[i].header)
		for _, r := range rows {
			w = max(w, len(cols[i].get(r)))
		}
		cols[i].w = w
	}
	keep := make([]bool, len(cols))
	for i := range keep {
		keep[i] = true
	}
	tableW := func() int {
		total, n := 0, 0
		for i, c := range cols {
			if keep[i] {
				total += c.w
				n++
			}
		}
		if n > 1 {
			total += 2 * (n - 1) // two spaces between columns
		}
		return total
	}
	for _, drop := range []int{5, 1} { // ALT first, then RAW
		if width <= 0 || tableW() <= width {
			break
		}
		keep[drop] = false
	}

	line := func(get func(int) string) string {
		var parts []string
		for i, c := range cols {
			if keep[i] {
				parts = append(parts, fmt.Sprintf("%-*s", c.w, get(i)))
			}
		}
		return strings.TrimRight(strings.Join(parts, "  "), " ")
	}

	var b strings.Builder
	b.WriteString(line(func(i int) string { return cols[i].header }))
	b.WriteByte('\n')
	b.WriteString(strings.Repeat("─", tableW()))
	for i := range rows {
		b.WriteByte('\n')
		r := rows[i]
		b.WriteString(line(func(j int) string { return cols[j].get(r) }))
	}
	return b.String()
}

// Render draws the table for one frame event. On a TTY it moves the cursor
// back up over the previous table and overwrites it; otherwise it prints each
// frame as a fresh block.
func (r *Renderer) Render(ev FrameEvent) {
	header := fmt.Sprintf("%s   frame %d   t=%.1fs   %s",
		r.title, ev.Index, ev.Elapsed.Seconds(), r.promMark(ev.Frame.Data))
	body := header + "\n" + renderTable(rowsFor(ev, r.def))

	if r.isTTY {
		if r.lastLines > 0 {
			fmt.Fprintf(r.w, "\033[%dA", r.lastLines) // cursor up over previous render
		}
		// Clear each line as we rewrite so shorter content can't leave artifacts.
		for line := range strings.SplitSeq(body, "\n") {
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
