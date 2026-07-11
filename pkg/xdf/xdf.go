// Package xdf reads TunerPro XDF bin-definition files just far enough to
// recover a table's shape: its title, dimensions, and X/Y axis labels. That is
// all the correction export needs — goaldl never reads or writes the bin
// itself, so Z-axis addresses, scaling equations, and edit metadata are
// deliberately out of scope (bin editing is TunerPro's half of the
// partnership; see product-knowledge/MISSION.md "Positioning").
//
// Two on-disk formats exist in the wild and both are supported behind one
// Parse entry point:
//
//   - the legacy text format (v1.x): "XDF" magic, %%TABLE%%…%%END%% blocks of
//     numeric-keyed lines — what tunerpro.net's own bin-definition library
//     ships for the GM $42 mask (data/xdf/42.xdf, our ground truth);
//   - the XML format (TunerPro v5+): <XDFFORMAT> with <XDFTABLE>/<XDFAXIS>
//     elements — what current community definitions use.
//
// The package imports no other goaldl package: it is a pure definition
// parser, the future ADX importer's sibling.
package xdf

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// Axis is one labelled table axis. Labels are the literal breakpoint values
// in file order; Units is the declared unit string ("RPM", "kPa", often "").
// Eq is the axis equation as written ("X" when absent) — anything but the
// identity means the displayed axis is computed, so the literal labels can't
// be trusted without evaluating math against the bin.
type Axis struct {
	Labels []float64
	Units  string
	Eq     string
	// LabelErr records labels that exist but couldn't be used (e.g. text
	// labels in an XML file). Empty when Labels is trustworthy.
	LabelErr string
}

// Table is one 2D table definition: X runs across the columns, Y down the
// rows (both formats share that convention). Rows/Cols are the declared Z
// dimensions and may disagree with the label counts in a malformed file —
// Find validates that before handing a Table to a caller.
type Table struct {
	Title string
	X, Y  Axis
	Rows  int
	Cols  int
}

// File is a parsed XDF: the format that was sniffed and every table block
// that carries at least one axis label. Category separators (the legacy
// format marks section headers like "     Fuel" as label-less pseudo-tables)
// are dropped at parse time so listing and matching never see them.
type File struct {
	Format string // "legacy" or "xml"
	Tables []Table
}

// ErrEmbeddedAxis marks a table whose axis breakpoints live in the bin (or
// are computed from it) rather than as literal labels in the XDF. Reading
// them would require the bin file, which goaldl deliberately does not do.
var ErrEmbeddedAxis = errors.New("axis is computed from the bin, not stored in the XDF; goaldl does not read bin files")

// ErrUnknownFormat is returned when the input is neither a legacy-text nor an
// XML XDF.
var ErrUnknownFormat = errors.New("not a TunerPro XDF (expected the legacy \"XDF\" text magic or an <XDFFORMAT> XML document)")

// NotFoundError reports a title that matched no table; Available carries
// every real table title so the caller can show the user what exists.
type NotFoundError struct {
	Title     string
	Available []string
}

func (e *NotFoundError) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("no table matching %q: the XDF contains no tables with axis labels", e.Title)
	}
	return fmt.Sprintf("no table matching %q; available tables:\n  %s",
		e.Title, strings.Join(e.Available, "\n  "))
}

// AmbiguousError reports a title that matched more than one table.
type AmbiguousError struct {
	Title      string
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches %d tables — be more specific:\n  %s",
		e.Title, len(e.Candidates), strings.Join(e.Candidates, "\n  "))
}

// Parse sniffs the format and parses the whole definition. The sniff is
// content-based, never extension-based: legacy files start with the literal
// magic "XDF" on the first line; XML files open with '<' (possibly after
// whitespace — TunerPro writes a leading <!-- timestamp --> comment).
func Parse(r io.Reader) (*File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading XDF: %w", err)
	}
	trimmed := strings.TrimLeft(string(data), " \t\r\n")
	switch {
	case strings.HasPrefix(trimmed, "<"):
		return parseXML(data)
	case strings.HasPrefix(trimmed, "XDF"):
		return parseLegacy(string(data))
	default:
		return nil, ErrUnknownFormat
	}
}

// Find locates one table by title. Matching is forgiving in the ways that
// help at a command line and strict in the ways that prevent surprises:
// whitespace-trimmed case-insensitive exact match first (a unique hit wins;
// duplicate titles in the file are ambiguous), then case-insensitive
// substring match (again only a unique hit wins). The returned table has
// passed Validate — a Find success is safe to build a grid on.
func (f *File) Find(title string) (*Table, error) {
	if len(f.Tables) == 0 {
		return nil, &NotFoundError{Title: title}
	}
	want := strings.ToLower(strings.TrimSpace(title))

	var exact, sub []*Table
	for i := range f.Tables {
		t := &f.Tables[i]
		got := strings.ToLower(strings.TrimSpace(t.Title))
		if got == want {
			exact = append(exact, t)
		} else if strings.Contains(got, want) {
			sub = append(sub, t)
		}
	}

	pick := exact
	if len(pick) == 0 {
		pick = sub
	}
	switch len(pick) {
	case 0:
		return nil, &NotFoundError{Title: title, Available: f.titles()}
	case 1:
		if err := pick[0].Validate(); err != nil {
			return nil, fmt.Errorf("table %q: %w", pick[0].Title, err)
		}
		return pick[0], nil
	default:
		var names []string
		for _, t := range pick {
			names = append(names, t.Title)
		}
		return nil, &AmbiguousError{Title: title, Candidates: names}
	}
}

func (f *File) titles() []string {
	var out []string
	for _, t := range f.Tables {
		out = append(out, t.Title)
	}
	return out
}

// Validate checks that the table's axes are usable as literal grid axes.
// Listing is deliberately lenient (a broken table still shows up in
// discovery so the user can see it exists); this gate runs when a table is
// actually selected.
func (t *Table) Validate() error {
	if t.Rows < 2 || t.Cols < 2 {
		return fmt.Errorf("not a 2D table (%d rows × %d cols); the correction export needs an RPM×MAP table", t.Rows, t.Cols)
	}
	if err := t.X.validate("X", t.Cols); err != nil {
		return err
	}
	return t.Y.validate("Y", t.Rows)
}

func (a *Axis) validate(name string, wantCount int) error {
	if a.LabelErr != "" {
		return fmt.Errorf("%s axis: %s", name, a.LabelErr)
	}
	if len(a.Labels) == 0 {
		return fmt.Errorf("%s axis: %w", name, ErrEmbeddedAxis)
	}
	if len(a.Labels) != wantCount {
		return fmt.Errorf("%s axis has %d labels but the table declares %d", name, len(a.Labels), wantCount)
	}
	allEqual, asc, desc := true, true, true
	for i := 1; i < len(a.Labels); i++ {
		if a.Labels[i] != a.Labels[0] {
			allEqual = false
		}
		if a.Labels[i] <= a.Labels[i-1] {
			asc = false
		}
		if a.Labels[i] >= a.Labels[i-1] {
			desc = false
		}
	}
	// All-equal labels (TunerPro writes "0.00" placeholders when the real
	// breakpoints live at a bin address) are the embedded-axis signature.
	if allEqual {
		return fmt.Errorf("%s axis labels are all %g: %w", name, a.Labels[0], ErrEmbeddedAxis)
	}
	if !asc && !desc {
		return fmt.Errorf("%s axis labels are not monotonic", name)
	}
	if eq := strings.TrimSpace(a.Eq); eq != "" && !strings.EqualFold(eq, "x") {
		return fmt.Errorf("%s axis has a non-identity equation %q: %w", name, a.Eq, ErrEmbeddedAxis)
	}
	return nil
}
