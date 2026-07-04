package blm

import (
	"fmt"
	"strings"
)

// RenderFloat formats a value matrix as a tab-separated "RPM \ MAP" table, the
// same shape as data/20250601_162123_BLM.txt. prec is the decimal precision.
func (g *Grid) RenderFloat(title string, m [][]float64, prec int) string {
	var b strings.Builder
	fmt.Fprintln(&b, title)
	writeHeader(&b, g.MAP)
	for r, rpm := range g.RPM {
		fmt.Fprintf(&b, "%g", rpm)
		for c := range g.MAP {
			fmt.Fprintf(&b, "\t%.*f", prec, m[r][c])
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// RenderInt formats an integer matrix (e.g. sample counts) as the same table.
func (g *Grid) RenderInt(title string, m [][]int) string {
	var b strings.Builder
	fmt.Fprintln(&b, title)
	writeHeader(&b, g.MAP)
	for r, rpm := range g.RPM {
		fmt.Fprintf(&b, "%g", rpm)
		for c := range g.MAP {
			fmt.Fprintf(&b, "\t%d", m[r][c])
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func writeHeader(b *strings.Builder, mapLabels []float64) {
	b.WriteString("RPM \\ MAP")
	for _, m := range mapLabels {
		fmt.Fprintf(b, "\t%g", m)
	}
	b.WriteByte('\n')
}
