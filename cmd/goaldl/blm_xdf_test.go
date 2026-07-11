package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/xdf"
)

// loadFixtureTable parses one of pkg/xdf's from-scratch fixtures and selects
// a table, so the command-layer tests run on exactly what the parser hands
// the command in production.
func loadFixtureTable(t *testing.T, fixture, title string) *xdf.Table {
	t.Helper()
	fh, err := os.Open(filepath.Join("..", "..", "pkg", "xdf", "testdata", fixture))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer fh.Close()
	f, err := xdf.Parse(fh)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tab, err := f.Find(title)
	if err != nil {
		t.Fatalf("Find(%q): %v", title, err)
	}
	return tab
}

func driveFrames(t *testing.T) []decoder.Frame {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "pkg", "decoder", "testdata", "drive_4800.raw"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return decoder.New(decoder.DefaultConfig()).Decode(raw)
}

// TestAccumulateOnXDFAxes drives the real capture into a grid built on the
// Main VE Table's axes (via the fixture XDF, which mirrors the official
// 42.xdf) and cross-checks against the default-axes regression numbers:
// same sample count, and the same 1600×40 cell — the VE table's kPa axis is
// identical to the default MAP axis and 1600 RPM exists on both RPM axes, so
// interior binning must agree exactly.
func TestAccumulateOnXDFAxes(t *testing.T) {
	tab := loadFixtureTable(t, "mini-legacy.xdf", "Main VE Table")
	axes, err := classifyAxes(tab)
	if err != nil {
		t.Fatalf("classifyAxes: %v", err)
	}
	if !axes.rpmIsY {
		t.Fatal("VE table should classify with RPM on Y")
	}

	frames := driveFrames(t)
	grid := blm.New(axes.rpm, axes.mapKPa)
	openLoop, blmOff, outOfRange := accumulateBLMInto(grid, frames)

	// Same gating as the default grid — axes don't change what records.
	if grid.TotalSamples() != 469 {
		t.Errorf("recorded %d, want 469", grid.TotalSamples())
	}
	if openLoop != 40 || blmOff != 126 {
		t.Errorf("skips = %d/%d, want 40/126", openLoop, blmOff)
	}
	// The drive reached 3700 RPM; the VE axis tops out at 3200, so some
	// samples must be flagged as absorbed by edge cells.
	if outOfRange == 0 {
		t.Error("outOfRange = 0; the drive exceeds the VE table's 3200 RPM top row")
	}

	// The WinALDL-cross-checked cell reproduces on the XDF axes.
	r, c := grid.Cell(1600, 40)
	if avg := grid.Average()[r][c]; math.Abs(avg-117.17) > 0.1 {
		t.Errorf("cell 1600/40 average = %.2f, want ~117.17", avg)
	}
}

// TestAccumulateBLMWrapperUnchanged pins the default-axes wrapper to the
// refactored implementation: identical results to the pre-XDF regression
// (which TestAccumulateBLM continues to assert independently).
func TestAccumulateBLMWrapperUnchanged(t *testing.T) {
	frames := driveFrames(t)
	grid, openLoop, blmOff := accumulateBLM(frames)
	grid2 := blm.NewDefault()
	openLoop2, blmOff2, _ := accumulateBLMInto(grid2, frames)
	if grid.TotalSamples() != grid2.TotalSamples() || openLoop != openLoop2 || blmOff != blmOff2 {
		t.Error("accumulateBLM wrapper diverges from accumulateBLMInto")
	}
}

func TestPasteBlockFormat(t *testing.T) {
	tab := loadFixtureTable(t, "mini-legacy.xdf", "Main VE Table")
	axes, err := classifyAxes(tab)
	if err != nil {
		t.Fatalf("classifyAxes: %v", err)
	}
	grid := blm.New(axes.rpm, axes.mapKPa)
	for range 5 {
		grid.Add(800, 40, 118) // trusted: 118/128 = 0.922
	}
	grid.Add(1600, 50, 100) // thin: held at 1.000

	block := pasteBlock(grid, axes, blm.DefaultMinSamples)
	lines := strings.Split(block, "\r\n")

	// Headerless: 8 RPM rows exactly, each 9 tab-separated %.3f values,
	// CRLF-terminated (so the split leaves one trailing empty element).
	if len(lines) != len(axes.rpm)+1 || lines[len(lines)-1] != "" {
		t.Fatalf("got %d CRLF lines, want %d", len(lines)-1, len(axes.rpm))
	}
	for i, l := range lines[:len(lines)-1] {
		if got := len(strings.Split(l, "\t")); got != len(axes.mapKPa) {
			t.Errorf("row %d has %d cells, want %d", i, got, len(axes.mapKPa))
		}
		if strings.ContainsAny(l, " ,") {
			t.Errorf("row %d contains non-TSV separators: %q", i, l)
		}
	}
	// Row 800 (index 1 on the VE RPM axis), col 40 kPa (index 2): 0.922.
	row800 := strings.Split(lines[1], "\t")
	if row800[2] != "0.922" {
		t.Errorf("trusted cell = %q, want 0.922", row800[2])
	}
	// The thin cell holds 1.000; so does every untouched cell.
	row1600 := strings.Split(lines[3], "\t")
	if row1600[3] != "1.000" {
		t.Errorf("thin cell = %q, want 1.000", row1600[3])
	}
	// No LF-only endings anywhere.
	if strings.Contains(strings.ReplaceAll(block, "\r\n", ""), "\n") {
		t.Error("paste block contains bare LF line endings")
	}
}

// TestPasteBlockTransposed checks the orientation contract: a table whose X
// axis is RPM and Y axis is MAP gets a block in the table's own layout (MAP
// rows × RPM columns), holding the same cell values.
func TestPasteBlockTransposed(t *testing.T) {
	tab := loadFixtureTable(t, "mini-legacy.xdf", "Transposed VE")
	axes, err := classifyAxes(tab)
	if err != nil {
		t.Fatalf("classifyAxes: %v", err)
	}
	if axes.rpmIsY {
		t.Fatal("transposed table should classify with RPM on X")
	}

	grid := blm.New(axes.rpm, axes.mapKPa) // grid stays RPM-rows internally
	for range 5 {
		grid.Add(1600, 60, 118) // RPM index 1 (of 800,1600,2400,3200), MAP index 1 (of 20,60,100)
	}
	block := pasteBlock(grid, axes, blm.DefaultMinSamples)
	lines := strings.Split(strings.TrimSuffix(block, "\r\n"), "\r\n")

	// Table layout: 3 MAP rows × 4 RPM cols.
	if len(lines) != 3 {
		t.Fatalf("got %d rows, want 3 (MAP rows)", len(lines))
	}
	for i, l := range lines {
		if got := len(strings.Split(l, "\t")); got != 4 {
			t.Errorf("row %d has %d cells, want 4 (RPM cols)", i, got)
		}
	}
	// The trusted cell lands at MAP row 1, RPM col 1.
	if cell := strings.Split(lines[1], "\t")[1]; cell != "0.922" {
		t.Errorf("transposed trusted cell = %q, want 0.922", cell)
	}
}

func TestClassifyAxesErrors(t *testing.T) {
	// Two MAP-like axes: must refuse rather than guess.
	_, err := classifyAxes(&xdf.Table{Title: "bad", Rows: 2, Cols: 2,
		X: xdf.Axis{Labels: []float64{20, 100}},
		Y: xdf.Axis{Labels: []float64{30, 90}}})
	if err == nil || !strings.Contains(err.Error(), "can't identify") {
		t.Errorf("two MAP axes: err = %v, want refusal", err)
	}
	// Units beat ranges: a 10–110 axis explicitly marked RPM classifies RPM.
	axes, err := classifyAxes(&xdf.Table{Title: "units", Rows: 2, Cols: 2,
		X: xdf.Axis{Labels: []float64{20, 100}, Units: "RPM"},
		Y: xdf.Axis{Labels: []float64{30, 90}, Units: "kPa"}})
	if err != nil {
		t.Fatalf("unit-labelled table: %v", err)
	}
	if axes.rpmIsY {
		t.Error("units say RPM is X")
	}
}

func TestListXDFTables(t *testing.T) {
	fh, err := os.Open(filepath.Join("..", "..", "pkg", "xdf", "testdata", "mini-legacy.xdf"))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer fh.Close()
	f, err := xdf.Parse(fh)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var buf bytes.Buffer
	listXDFTables(&buf, "mini-legacy.xdf", f)
	out := buf.String()
	for _, want := range []string{"legacy format", "Main VE Table", "8×9", "X kPa × Y RPM", "-table"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing missing %q:\n%s", want, out)
		}
	}
}
