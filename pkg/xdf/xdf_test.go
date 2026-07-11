package xdf

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// veRPM / veMAP are the $42 Main VE Table axes, as they appear in both the
// official legacy 42.xdf and the fixtures derived from its structure.
var (
	veRPM = []float64{400, 800, 1200, 1600, 2000, 2400, 2800, 3200}
	veMAP = []float64{20, 30, 40, 50, 60, 70, 80, 90, 100}
)

func parseFixture(t *testing.T, name string) *File {
	t.Helper()
	fh, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer fh.Close()
	f, err := Parse(fh)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return f
}

func TestParseLegacyFixture(t *testing.T) {
	f := parseFixture(t, "mini-legacy.xdf")
	if f.Format != "legacy" {
		t.Fatalf("Format = %q, want legacy", f.Format)
	}
	// The "     Fuel" category pseudo-table (no labels) must not list.
	want := []string{"Main VE Table", "VE Adder Table", "Transposed VE", "Scaled Axis Table", "One D Table"}
	if got := f.titles(); !reflect.DeepEqual(got, want) {
		t.Fatalf("titles = %v, want %v", got, want)
	}

	ve, err := f.Find("Main VE Table")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if ve.Rows != 8 || ve.Cols != 9 {
		t.Fatalf("dims = %d×%d, want 8×9", ve.Rows, ve.Cols)
	}
	if !reflect.DeepEqual(ve.Y.Labels, veRPM) || !reflect.DeepEqual(ve.X.Labels, veMAP) {
		t.Fatalf("axes = Y%v X%v", ve.Y.Labels, ve.X.Labels)
	}
	if ve.X.Units != "kPa" || ve.Y.Units != "RPM" {
		t.Fatalf("units = X%q Y%q", ve.X.Units, ve.Y.Units)
	}
}

func TestParseXMLFixture(t *testing.T) {
	f := parseFixture(t, "mini-xml.xdf")
	if f.Format != "xml" {
		t.Fatalf("Format = %q, want xml", f.Format)
	}
	ve, err := f.Find("main ve") // forgiving: substring, case-insensitive
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if ve.Rows != 8 || ve.Cols != 9 {
		t.Fatalf("dims = %d×%d, want 8×9", ve.Rows, ve.Cols)
	}
	if !reflect.DeepEqual(ve.Y.Labels, veRPM) || !reflect.DeepEqual(ve.X.Labels, veMAP) {
		t.Fatalf("axes = Y%v X%v", ve.Y.Labels, ve.X.Labels)
	}
}

func TestFindMatching(t *testing.T) {
	f := parseFixture(t, "mini-legacy.xdf")

	// Substring matching two tables ("Main VE Table", "VE Adder Table",
	// "Transposed VE") is ambiguous, and the error names the candidates.
	_, err := f.Find("ve")
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("Find(ve) err = %v, want AmbiguousError", err)
	}
	if len(amb.Candidates) != 3 {
		t.Fatalf("candidates = %v, want 3", amb.Candidates)
	}

	// No match lists everything available.
	_, err = f.Find("spark advance")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Find err = %v, want NotFoundError", err)
	}
	if len(nf.Available) != 5 {
		t.Fatalf("available = %v, want 5 titles", nf.Available)
	}

	// Exact match beats substring ambiguity: "Transposed VE" is exact even
	// though "ve" alone is ambiguous.
	if _, err := f.Find("transposed ve"); err != nil {
		t.Fatalf("exact match failed: %v", err)
	}

	// An empty file reports the distinct no-tables error.
	empty := &File{}
	_, err = empty.Find("anything")
	if !errors.As(err, &nf) || len(nf.Available) != 0 || !strings.Contains(err.Error(), "no tables") {
		t.Fatalf("empty Find err = %v, want no-tables NotFoundError", err)
	}
}

func TestValidateRejections(t *testing.T) {
	legacy := parseFixture(t, "mini-legacy.xdf")
	xml := parseFixture(t, "mini-xml.xdf")

	for _, tc := range []struct {
		file    *File
		title   string
		wantSub string // substring of the expected error
	}{
		{legacy, "Scaled Axis Table", "non-identity equation"},
		{legacy, "One D Table", "not a 2D table"},
		{xml, "Embedded Axis Table", "computed from the bin"},
		{xml, "Text Label Table", "not numeric"},
	} {
		_, err := tc.file.Find(tc.title)
		if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
			t.Errorf("Find(%q) err = %v, want %q", tc.title, err, tc.wantSub)
		}
	}

	// The two embedded-axis flavors both unwrap to ErrEmbeddedAxis so the
	// command layer can special-case the explanation.
	_, err := xml.Find("Embedded Axis Table")
	if !errors.Is(err, ErrEmbeddedAxis) {
		t.Errorf("embedded axis err = %v, want errors.Is ErrEmbeddedAxis", err)
	}
	_, err = legacy.Find("Scaled Axis Table")
	if !errors.Is(err, ErrEmbeddedAxis) {
		t.Errorf("scaled axis err = %v, want errors.Is ErrEmbeddedAxis", err)
	}
}

func TestParseErrors(t *testing.T) {
	// Unknown format.
	if _, err := Parse(strings.NewReader("hello world")); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("plain text err = %v, want ErrUnknownFormat", err)
	}
	// Broken XML aborts with a parse error.
	if _, err := Parse(strings.NewReader("<XDFFORMAT><XDFTABLE>")); err == nil {
		t.Error("broken XML parsed without error")
	}
	// A bad label in a legacy table doesn't abort the file (a defect in one
	// table must not hide the rest); it surfaces with a line number when the
	// table is selected. The official 42.xdf needs this: its 1D tables write
	// "XLabels =(null)".
	bad := "XDF\n1.1\n%%TABLE%%\n\t040005 Title =\"T\"\n\t040300 Rows =0x2\n\t040305 Cols =0x2\n" +
		"\t040350 XLabels =20,notanumber\n\t040360 YLabels =1,2\n%%END%%\n"
	f, err := Parse(strings.NewReader(bad))
	if err != nil {
		t.Fatalf("bad-label file should still parse: %v", err)
	}
	_, err = f.Find("T")
	if err == nil || !strings.Contains(err.Error(), "line 7") {
		t.Errorf("Find on bad-label table = %v, want line 7 reference", err)
	}
	// "(null)" labels mean no labels: the table is a pseudo/1D entry.
	null := "XDF\n1.1\n%%TABLE%%\n\t040005 Title =\"N\"\n\t040350 XLabels =(null)\n%%END%%\n"
	f, err = Parse(strings.NewReader(null))
	if err != nil || len(f.Tables) != 0 {
		t.Errorf("(null)-label table: err=%v tables=%v, want clean parse with no tables", err, f.titles())
	}
}

func TestNonMonotonicLabels(t *testing.T) {
	tab := Table{Title: "t", Rows: 2, Cols: 3,
		X: Axis{Labels: []float64{1, 3, 2}},
		Y: Axis{Labels: []float64{1, 2}}}
	if err := tab.Validate(); err == nil || !strings.Contains(err.Error(), "monotonic") {
		t.Errorf("Validate = %v, want monotonic error", err)
	}
	// Descending is legitimate (some definitions run axes high→low).
	tab.X.Labels = []float64{3, 2, 1}
	if err := tab.Validate(); err != nil {
		t.Errorf("descending axis rejected: %v", err)
	}
	// Label count must match the declared dimension.
	tab.X.Labels = []float64{3, 2}
	if err := tab.Validate(); err == nil || !strings.Contains(err.Error(), "declares") {
		t.Errorf("count mismatch = %v, want declares error", err)
	}
}

// TestRealXDF parses the real $42 definitions when present locally
// (data/xdf/ is gitignored pending license review, so CI skips these; the
// from-scratch fixtures above mirror the structures and keep equivalent
// assertions live everywhere). Three definitions, three community names for
// the same VE table — the reason -table is explicit rather than guessed —
// and all three must yield the identical 8×9 axes.
func TestRealXDF(t *testing.T) {
	for _, tc := range []struct {
		file    string
		format  string
		veTitle string
		tables  int
	}{
		{"42.xdf", "legacy", "Main VE Table", 50},                             // official tunerpro.net (text v1.1)
		{"$42-1227747-V5.9.3.xdf", "xml", "Fuel VE 1 - Main Fuel Table", 57}, // community, gearhead-efi thread 304
		{"$42-1227747-V4T.xdf", "xml", "VE as % (FL1)", 48},                  // community, TH400/3-speed variant
	} {
		t.Run(tc.file, func(t *testing.T) {
			fh, err := os.Open("../../data/xdf/" + tc.file)
			if err != nil {
				t.Skipf("real XDF not present (%v) — fixtures cover the structure", err)
			}
			defer fh.Close()

			f, err := Parse(fh)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if f.Format != tc.format {
				t.Fatalf("Format = %q, want %s", f.Format, tc.format)
			}
			if len(f.Tables) != tc.tables {
				t.Errorf("tables = %d, want %d", len(f.Tables), tc.tables)
			}
			ve, err := f.Find(tc.veTitle)
			if err != nil {
				t.Fatalf("Find(%q): %v", tc.veTitle, err)
			}
			if ve.Rows != 8 || ve.Cols != 9 {
				t.Fatalf("dims = %d×%d, want 8×9", ve.Rows, ve.Cols)
			}
			if !reflect.DeepEqual(ve.Y.Labels, veRPM) || !reflect.DeepEqual(ve.X.Labels, veMAP) {
				t.Fatalf("axes = Y%v X%v, want the documented VE axes", ve.Y.Labels, ve.X.Labels)
			}
			// "ve" alone must stay ambiguous in every real file (main +
			// adder/corrected variants all match).
			if _, err := f.Find("ve"); err == nil {
				t.Error("Find(ve) should be ambiguous")
			}
		})
	}
}
