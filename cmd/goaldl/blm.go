package main

import (
	"flag"
	"fmt"
	"os"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
)

// GM 1227747 (A033) frame offsets and MWAF1 flag bits, per data/A033.ads.
const (
	offMAP   = 6  // MAP sensor, raw byte; volts = raw * 0.0196
	offRPM   = 7  // engine speed; RPM = raw * 25
	offMWAF1 = 14 // mode/status word carrying the closed-loop and BLM-enable bits
	offBLM   = 18 // Block Learn Multiplier

	bitBLMEnable  = 1 // MWAF1 bit 1: block learn enabled
	bitClosedLoop = 7 // MWAF1 bit 7: loop status (1 = CLOSED)
)

// mapVoltsToKPa converts the A033 MAP sensor voltage to manifold pressure.
//
// ASSUMPTION — VERIFY against WinALDL: A033.ads reports MAP only in volts, so
// this uses the standard GM 1-bar MAP transfer (~1V≈20 kPa idle vacuum,
// ~4.9V≈105 kPa near WOT). If your WinALDL kPa column disagrees, adjust the
// slope/offset here; the binning and correction math are unaffected.
func mapVoltsToKPa(v float64) float64 {
	const slope, offset = 21.25, -1.25
	return slope*v + offset
}

// cmdBLM builds a BLM (fuel-trim) table from a capture, showing where the tune
// runs rich or lean across RPM and load. Only closed-loop, block-learn-enabled
// frames are recorded — BLM is frozen and meaningless otherwise.
func cmdBLM() {
	fs := flag.NewFlagSet("blm", flag.ExitOnError)
	baudRate := fs.Int("b", 4800, "UART sampling baud rate the capture was recorded at")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	minSamples := fs.Int("min", 1, "Hide cells with fewer than this many samples in the correction table")
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
		mwaf1 := f.Data[offMWAF1]
		closedLoop := (mwaf1>>bitClosedLoop)&1 == 1
		blmEnabled := (mwaf1>>bitBLMEnable)&1 == 1
		if !closedLoop {
			openLoop++
			continue
		}
		if !blmEnabled {
			blmOff++
			continue
		}
		rpm := float64(f.Data[offRPM]) * 25
		mapKPa := mapVoltsToKPa(float64(f.Data[offMAP]) * 0.0196)
		grid.Add(rpm, mapKPa, float64(f.Data[offBLM]))
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

	fmt.Print(grid.RenderInt("Samples", grid.Samples()))
	fmt.Println()
	fmt.Print(grid.RenderFloat("Average BLM (128 = neutral; <128 base tune rich, >128 lean)", grid.Average(), 1))
	fmt.Println()
	fmt.Print(grid.RenderFloat("Correction factor (multiply base VE/fuel by this)", correctionMasked(grid, *minSamples), 3))

	if *csvOut != "" {
		if err := writeCorrectionCSV(*csvOut, grid, *minSamples); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *csvOut, err)
			os.Exit(1)
		}
		fmt.Printf("\nWrote correction table to %s\n", *csvOut)
	}
}

// correctionMasked returns the correction grid with cells below minSamples
// forced to 1.0 (no change), so sparse, noisy cells don't suggest edits.
func correctionMasked(g *blm.Grid, minSamples int) [][]float64 {
	corr := g.Correction()
	samples := g.Samples()
	for r := range corr {
		for c := range corr[r] {
			if samples[r][c] < minSamples {
				corr[r][c] = 1.0
			}
		}
	}
	return corr
}

func writeCorrectionCSV(path string, g *blm.Grid, minSamples int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	corr := correctionMasked(g, minSamples)
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
