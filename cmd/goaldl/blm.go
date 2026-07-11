package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/xdf"
)

// accumulateBLM bins the recordable frames into a fresh default-axes grid.
// Kept as a thin wrapper so the pre-XDF call sites and tests are untouched.
func accumulateBLM(frames []decoder.Frame) (grid *blm.Grid, openLoop, blmOff int) {
	grid = blm.NewDefault()
	openLoop, blmOff, _ = accumulateBLMInto(grid, frames)
	return grid, openLoop, blmOff
}

// accumulateBLMInto bins the recordable frames into the given grid and
// reports how many frames were skipped because the ECM was in open loop or
// had block learn disabled (BLM is frozen and meaningless in those states).
// outOfRange counts recorded samples that fell beyond the grid's axis label
// range on either axis — they still bin to the nearest edge cell (Grid.Add's
// nearest-label semantics), but on narrow axes like a VE table's 400–3200 RPM
// the caller should tell the user the edges absorbed out-of-range driving.
// Pure and side-effect-free so it can be tested against a capture fixture.
func accumulateBLMInto(grid *blm.Grid, frames []decoder.Frame) (openLoop, blmOff, outOfRange int) {
	for _, f := range frames {
		ft := ecm.FuelTrimSample(f.Data)
		switch {
		case !ft.ClosedLoop:
			openLoop++
		case !ft.BLMEnabled:
			blmOff++
		default:
			if beyondAxis(grid.RPM, ft.RPM) || beyondAxis(grid.MAP, ft.MapKPa) {
				outOfRange++
			}
			grid.Add(ft.RPM, ft.MapKPa, ft.BLM)
		}
	}
	return openLoop, blmOff, outOfRange
}

// beyondAxis reports whether v lies outside the label range (labels may run
// ascending or descending; compare against both ends).
func beyondAxis(labels []float64, v float64) bool {
	lo, hi := labels[0], labels[len(labels)-1]
	if lo > hi {
		lo, hi = hi, lo
	}
	return v < lo || v > hi
}

// cmdBLM builds a BLM (fuel-trim) table from a capture, showing where the tune
// runs rich or lean across RPM and load. It records every closed-loop,
// block-learn-enabled frame (BLM is frozen and meaningless otherwise) and
// reports the "Wide Average" per cell — the mean BLM over all such samples.
// Target is 128: above 128 the cell ran lean, below it ran rich.
//
// With -xdf, the grid is built on a table's own axes straight out of the
// TunerPro definition instead of the WinALDL-style defaults, and a paste
// block is emitted that drops onto that table in TunerPro via
// paste-with-multiply — the drive→tune loop with no manual re-binning.
func cmdBLM(args []string) {
	fs := flag.NewFlagSet("blm", flag.ExitOnError)
	baudRate := fs.Int("b", 4800, "UART sampling baud rate the capture was recorded at")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	minSamples := fs.Int("min", blm.DefaultMinSamples, "Samples a cell needs before its correction is trusted (below this: no change)")
	csvOut := fs.String("o", "", "Write the correction table to this CSV file")
	xdfPath := fs.String("xdf", "", "TunerPro XDF definition; grid axes come from the -table table (alone: list tables)")
	tableName := fs.String("table", "", "Table title in the -xdf file to take axes from (forgiving match)")
	pasteOut := fs.String("paste", "", "Write the TunerPro paste block to this file (requires -xdf and -table)")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: goaldl blm <capture.raw> [-o correction.csv] [-xdf 42.xdf -table \"Main VE Table\" [-paste ve.txt]]")
		os.Exit(1)
	}
	inName := fs.Arg(0)
	fs.Parse(fs.Args()[1:]) // allow flags after the filename

	if *xdfPath == "" && (*tableName != "" || *pasteOut != "") {
		fmt.Fprintln(os.Stderr, "-table and -paste need -xdf (the TunerPro definition the axes come from)")
		os.Exit(1)
	}
	if *pasteOut != "" && *tableName == "" {
		// Without this, -paste would fall silently into the discovery
		// listing below and the requested file would never be written.
		fmt.Fprintln(os.Stderr, "-paste needs -table (which table's layout should the paste block use?)")
		os.Exit(1)
	}

	// -xdf without -table is discovery: show what the definition offers.
	if *xdfPath != "" && *tableName == "" {
		file, err := parseXDF(*xdfPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		listXDFTables(os.Stdout, *xdfPath, file)
		return
	}

	var axes *xdfAxes
	if *xdfPath != "" {
		file, err := parseXDF(*xdfPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		table, err := file.Find(*tableName)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		axes, err = classifyAxes(table)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	raw, err := os.ReadFile(inName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", inName, err)
		os.Exit(1)
	}

	cfg := decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert}
	frames := decoder.New(cfg).Decode(raw)
	if len(frames) == 0 {
		fmt.Fprintln(os.Stderr, "No frames decoded; check baud rate / capture (try goaldl decode first).")
		os.Exit(1)
	}

	grid := blm.NewDefault()
	if axes != nil {
		grid = blm.New(axes.rpm, axes.mapKPa)
	}
	openLoop, blmOff, outOfRange := accumulateBLMInto(grid, frames)

	fmt.Printf("Decoded %d frames from %s\n", len(frames), inName)
	fmt.Printf("Recorded %d into BLM cells (skipped %d open-loop, %d block-learn-disabled)\n\n",
		grid.TotalSamples(), openLoop, blmOff)
	if grid.TotalSamples() == 0 {
		fmt.Println("No closed-loop, block-learn-enabled frames — nothing to map.")
		fmt.Println("This is expected for a cold or wide-open-throttle capture; BLM only")
		fmt.Println("learns once the engine is warm and in closed loop.")
		return
	}

	if axes != nil {
		fmt.Println(axes.describe(*xdfPath))
		if outOfRange > 0 {
			fmt.Printf("Note: %d samples fell outside the table's axis range and were absorbed into edge cells.\n", outOfRange)
		}
		fmt.Println()
	}

	fmt.Printf("%d of %d cells reached %d+ samples (trusted)\n\n",
		grid.PopulatedCells(*minSamples), grid.PopulatedCells(1), *minSamples)

	fmt.Print(grid.RenderInt("Samples", grid.Samples()))
	fmt.Println()
	fmt.Print(grid.RenderFloat("Wide Average BLM (target 128; >128 lean, <128 rich)", grid.Average(), 1))
	fmt.Println()
	fmt.Printf("Correction factor = avg/128 (cells with <%d samples held at 1.000)\n", *minSamples)
	fmt.Print(grid.RenderFloat("", grid.CorrectionAtLeast(*minSamples), 3))

	if *csvOut != "" {
		f, err := os.Create(*csvOut)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", *csvOut, err)
			os.Exit(1)
		}
		writeCorrectionCSV(f, grid, *minSamples)
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *csvOut, err)
			os.Exit(1)
		}
		fmt.Printf("\nWrote correction table to %s\n", *csvOut)
	}

	if axes != nil {
		block := pasteBlock(grid, axes, *minSamples)
		fmt.Printf("\n--- TunerPro paste block (%q — select the whole table, paste with multiply) ---\n", axes.title)
		fmt.Print(block)
		fmt.Println("--- end paste block ---")
		if *pasteOut != "" {
			if err := os.WriteFile(*pasteOut, []byte(block), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *pasteOut, err)
				os.Exit(1)
			}
			fmt.Printf("Wrote paste block to %s\n", *pasteOut)
		}
	}
}

func parseXDF(path string) (*xdf.File, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening XDF: %w", err)
	}
	defer fh.Close()
	file, err := xdf.Parse(fh)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

// listXDFTables prints the discovery view: every table the definition offers,
// with enough shape/unit information to pick a -table title.
func listXDFTables(w io.Writer, path string, f *xdf.File) {
	fmt.Fprintf(w, "%s (%s format): %d tables\n", path, f.Format, len(f.Tables))
	for _, t := range f.Tables {
		units := ""
		if t.X.Units != "" || t.Y.Units != "" {
			units = fmt.Sprintf("  [X %s × Y %s]", orDash(t.X.Units), orDash(t.Y.Units))
		}
		fmt.Fprintf(w, "  %-45q %d×%d%s\n", t.Title, t.Rows, t.Cols, units)
	}
	fmt.Fprintln(w, "\nUse -table \"<title>\" to build the correction grid on a table's axes.")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// xdfAxes is a table's axes with their roles identified: which one is RPM and
// which is MAP, and whether RPM runs down the rows (rpmIsY) — needed so the
// paste block can be emitted in the table's own layout even when a definition
// transposes the conventional orientation.
type xdfAxes struct {
	title       string
	rpm, mapKPa []float64
	rpmIsY      bool
}

// classifyAxes decides which of the table's axes is RPM and which is MAP.
// Declared units win (a definition that says "RPM" means it); ranges are the
// fallback for unit-less definitions. Refusing to guess is a feature: binning
// fuel trim into a transposed or non-RPM×MAP table would produce a
// plausible-looking but wrong export, which is worse than an error.
func classifyAxes(t *xdf.Table) (*xdfAxes, error) {
	xRole := axisRole(t.X)
	yRole := axisRole(t.Y)
	switch {
	case xRole == "map" && yRole == "rpm":
		return &xdfAxes{title: t.Title, rpm: t.Y.Labels, mapKPa: t.X.Labels, rpmIsY: true}, nil
	case xRole == "rpm" && yRole == "map":
		return &xdfAxes{title: t.Title, rpm: t.X.Labels, mapKPa: t.Y.Labels, rpmIsY: false}, nil
	default:
		return nil, fmt.Errorf(
			"table %q: can't identify an RPM axis and a MAP axis (X: units %q, %s; Y: units %q, %s) — the correction export needs an RPM×MAP table",
			t.Title, t.X.Units, describeRange(t.X.Labels), t.Y.Units, describeRange(t.Y.Labels))
	}
}

func describeRange(labels []float64) string {
	if len(labels) == 0 {
		return "no labels"
	}
	return fmt.Sprintf("range %g–%g", labels[0], labels[len(labels)-1])
}

// axisRole classifies one axis as "rpm", "map", or "" (unknown). MAP is
// checked before RPM in the range fallback because every plausible kPa range
// (10–110) also fits inside a plausible RPM range, not vice versa.
func axisRole(a xdf.Axis) string {
	u := strings.ToLower(a.Units)
	switch {
	case strings.Contains(u, "rpm"):
		return "rpm"
	case strings.Contains(u, "kpa"):
		return "map"
	}
	if len(a.Labels) == 0 {
		return ""
	}
	lo, hi := a.Labels[0], a.Labels[len(a.Labels)-1]
	if lo > hi {
		lo, hi = hi, lo
	}
	switch {
	case lo >= 10 && hi <= 110:
		return "map"
	case lo >= 0 && hi > 300 && hi <= 8000:
		return "rpm"
	}
	return ""
}

func (a *xdfAxes) describe(path string) string {
	rows, cols := "RPM", "kPa"
	rowLabels, colLabels := a.rpm, a.mapKPa
	if !a.rpmIsY {
		rows, cols = cols, rows
		rowLabels, colLabels = colLabels, rowLabels
	}
	return fmt.Sprintf("Axes from %q (%s): rows %s %g–%g, cols %s %g–%g",
		a.title, path,
		rows, rowLabels[0], rowLabels[len(rowLabels)-1],
		cols, colLabels[0], colLabels[len(colLabels)-1])
}

// pasteBlock renders the correction grid as TunerPro's clipboard shape: a
// headerless tab-separated block of %.3f values with CRLF line endings, laid
// out exactly like the table (rows = the table's Y axis) so a select-all +
// paste-with-multiply lands each factor on its cell. Cells below minSamples
// emit 1.000 (no change) — the same confidence rule as every other view.
func pasteBlock(g *blm.Grid, axes *xdfAxes, minSamples int) string {
	corr := g.CorrectionAtLeast(minSamples) // [rpm row][map col]
	var b strings.Builder
	if axes.rpmIsY {
		for r := range g.RPM {
			writePasteRow(&b, len(g.MAP), func(c int) float64 { return corr[r][c] })
		}
	} else {
		// Transposed table: its rows are MAP, its columns RPM.
		for c := range g.MAP {
			writePasteRow(&b, len(g.RPM), func(r int) float64 { return corr[r][c] })
		}
	}
	return b.String()
}

func writePasteRow(b *strings.Builder, n int, cell func(int) float64) {
	for i := range n {
		if i > 0 {
			b.WriteByte('\t')
		}
		fmt.Fprintf(b, "%.3f", cell(i))
	}
	b.WriteString("\r\n")
}

// writeCorrectionCSV writes the correction grid as CSV (RPM rows, MAP columns),
// with cells below minSamples held at 1.000.
func writeCorrectionCSV(w io.Writer, g *blm.Grid, minSamples int) {
	corr := g.CorrectionAtLeast(minSamples)
	fmt.Fprint(w, "rpm\\map")
	for _, m := range g.MAP {
		fmt.Fprintf(w, ",%g", m)
	}
	fmt.Fprintln(w)
	for r, rpm := range g.RPM {
		fmt.Fprintf(w, "%g", rpm)
		for c := range g.MAP {
			fmt.Fprintf(w, ",%.3f", corr[r][c])
		}
		fmt.Fprintln(w)
	}
}
