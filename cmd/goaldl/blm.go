package main

import (
	"flag"
	"fmt"
	"os"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
)

// cmdBLM builds a BLM (fuel-trim) table from a capture, showing where the tune
// runs rich or lean across RPM and load. It records every closed-loop,
// block-learn-enabled frame (BLM is frozen and meaningless otherwise) and
// reports the "Wide Average" per cell — the mean BLM over all such samples.
// Target is 128: above 128 the cell ran lean, below it ran rich.
func cmdBLM() {
	fs := flag.NewFlagSet("blm", flag.ExitOnError)
	baudRate := fs.Int("b", 4800, "UART sampling baud rate the capture was recorded at")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	minSamples := fs.Int("min", blm.DefaultMinSamples, "Samples a cell needs before its correction is trusted (below this: no change)")
	csvOut := fs.String("o", "", "Write the correction table to this CSV file")
	fs.Parse(os.Args[2:])

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: goaldl blm <capture.raw> [-o correction.csv]")
		os.Exit(1)
	}
	inName := fs.Arg(0)
	fs.Parse(fs.Args()[1:]) // allow flags after the filename

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
	var openLoop, blmOff int
	for _, f := range frames {
		ft := ecm.FuelTrimSample(f.Data)
		switch {
		case !ft.ClosedLoop:
			openLoop++
		case !ft.BLMEnabled:
			blmOff++
		default:
			grid.Add(ft.RPM, ft.MapKPa, ft.BLM)
		}
	}

	fmt.Printf("Decoded %d frames from %s\n", len(frames), inName)
	fmt.Printf("Recorded %d into BLM cells (skipped %d open-loop, %d block-learn-disabled)\n\n",
		grid.TotalSamples(), openLoop, blmOff)
	if grid.TotalSamples() == 0 {
		fmt.Println("No closed-loop, block-learn-enabled frames — nothing to map.")
		fmt.Println("This is expected for a cold or wide-open-throttle capture; BLM only")
		fmt.Println("learns once the engine is warm and in closed loop.")
		return
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
		if err := writeCorrectionCSV(*csvOut, grid, *minSamples); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *csvOut, err)
			os.Exit(1)
		}
		fmt.Printf("\nWrote correction table to %s\n", *csvOut)
	}
}

func writeCorrectionCSV(path string, g *blm.Grid, minSamples int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	corr := g.CorrectionAtLeast(minSamples)
	fmt.Fprint(f, "rpm\\map")
	for _, m := range g.MAP {
		fmt.Fprintf(f, ",%g", m)
	}
	fmt.Fprintln(f)
	for r, rpm := range g.RPM {
		fmt.Fprintf(f, "%g", rpm)
		for c := range g.MAP {
			fmt.Fprintf(f, ",%.3f", corr[r][c])
		}
		fmt.Fprintln(f)
	}
	return nil
}
