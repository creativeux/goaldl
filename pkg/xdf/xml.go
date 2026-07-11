package xdf

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// XML XDF (TunerPro v5+): <XDFFORMAT> wrapping <XDFTABLE> elements, each with
// a <title> and three <XDFAXIS> children (id="x", "y", "z"). Axis breakpoints
// are <LABEL index value> children; the Z axis's <EMBEDDEDDATA> carries the
// table dimensions (mmedrowcount/mmedcolcount). Structure confirmed against a
// real community definition (see the feature trace); the field subset here is
// deliberately the same one the legacy parser extracts.
type xmlFormat struct {
	Tables []xmlTable `xml:"XDFTABLE"`
}

type xmlTable struct {
	Title string    `xml:"title"`
	Axes  []xmlAxis `xml:"XDFAXIS"`
}

type xmlAxis struct {
	ID       string     `xml:"id,attr"`
	Units    string     `xml:"units"`
	Count    int        `xml:"indexcount"`
	Labels   []xmlLabel `xml:"LABEL"`
	Math     xmlMath    `xml:"MATH"`
	Embedded xmlEmbed   `xml:"EMBEDDEDDATA"`
}

type xmlLabel struct {
	Index int    `xml:"index,attr"`
	Value string `xml:"value,attr"`
}

type xmlMath struct {
	Equation string `xml:"equation,attr"`
}

type xmlEmbed struct {
	Rows int `xml:"mmedrowcount,attr"`
	Cols int `xml:"mmedcolcount,attr"`
}

func parseXML(data []byte) (*File, error) {
	var doc xmlFormat
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing XML XDF: %w", err)
	}
	f := &File{Format: "xml"}
	for _, xt := range doc.Tables {
		t := Table{Title: xt.Title}
		var xCount, yCount int
		for _, ax := range xt.Axes {
			switch strings.ToLower(ax.ID) {
			case "x":
				t.X, xCount = xmlToAxis(ax), ax.Count
			case "y":
				t.Y, yCount = xmlToAxis(ax), ax.Count
			case "z":
				t.Rows, t.Cols = ax.Embedded.Rows, ax.Embedded.Cols
			}
		}
		// Older writers omit the Z dimensions; the x/y indexcounts carry the
		// same information.
		if t.Rows == 0 {
			t.Rows = yCount
		}
		if t.Cols == 0 {
			t.Cols = xCount
		}
		// As in the legacy parser: label-less on both axes means the entry
		// carries nothing we can build a grid from (pure embedded-axis
		// tables still list — their labels exist but fail Validate later).
		if len(t.X.Labels) > 0 || len(t.Y.Labels) > 0 {
			f.Tables = append(f.Tables, t)
		}
	}
	return f, nil
}

// xmlToAxis converts one axis element. Non-numeric labels (the format allows
// free text) don't abort the parse — the table still lists in discovery; the
// defect is recorded on the axis and surfaces if that table is selected.
func xmlToAxis(ax xmlAxis) Axis {
	labels := append([]xmlLabel(nil), ax.Labels...)
	sort.SliceStable(labels, func(i, j int) bool { return labels[i].Index < labels[j].Index })
	a := Axis{Units: strings.TrimSpace(ax.Units), Eq: strings.TrimSpace(ax.Math.Equation)}
	for _, l := range labels {
		v, err := strconv.ParseFloat(strings.TrimSpace(l.Value), 64)
		if err != nil {
			return Axis{Units: a.Units, Eq: a.Eq, LabelErr: fmt.Sprintf("label %d is not numeric (%q)", l.Index, l.Value)}
		}
		a.Labels = append(a.Labels, v)
	}
	return a
}
