package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
)

// TestAccumulateBLM is an end-to-end regression over the real drive capture:
// decode → gate → bin, asserting the recorded/skipped counts and a couple of
// specific cell values. Any change to decoding, the closed-loop/BLM-enable
// gating, the MAP transfer, or the averaging will move these and fail here.
func TestAccumulateBLM(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "pkg", "decoder", "testdata", "drive_4800.raw"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	frames := decoder.New(decoder.DefaultConfig()).Decode(raw)

	grid, openLoop, blmOff := accumulateBLM(frames)

	if grid.TotalSamples() != 469 {
		t.Errorf("recorded %d, want 469", grid.TotalSamples())
	}
	if openLoop != 40 {
		t.Errorf("open-loop skipped %d, want 40", openLoop)
	}
	if blmOff != 126 {
		t.Errorf("block-learn-disabled skipped %d, want 126", blmOff)
	}
	// Every recorded frame is accounted for.
	if got := grid.TotalSamples() + openLoop + blmOff; got != len(frames) {
		t.Errorf("recorded+skipped = %d, want %d frames", got, len(frames))
	}
	if pop := grid.PopulatedCells(blm.DefaultMinSamples); pop != 25 {
		t.Errorf("populated cells (>=4) = %d, want 25", pop)
	}

	// Cell 1600 RPM × 40 kPa: average ~116.0, correction ~0.906 (116/128).
	r, c := grid.Cell(1600, 40)
	if avg := grid.Average()[r][c]; math.Abs(avg-116.0) > 0.1 {
		t.Errorf("cell 1600/40 average = %.2f, want ~116.0", avg)
	}
	if corr := grid.CorrectionAtLeast(blm.DefaultMinSamples)[r][c]; math.Abs(corr-0.906) > 0.002 {
		t.Errorf("cell 1600/40 correction = %.3f, want ~0.906", corr)
	}
}

func TestWriteCorrectionCSV(t *testing.T) {
	g := blm.NewDefault()
	// One trusted cell (>=4 samples) and one thin cell (below threshold).
	for i := 0; i < 5; i++ {
		g.Add(800, 40, 118) // 800×40: avg 118 → 118/128 = 0.922
	}
	g.Add(1200, 50, 100) // 1200×50: only 1 sample → held at 1.000

	var buf bytes.Buffer
	writeCorrectionCSV(&buf, g, blm.DefaultMinSamples)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	if lines[0] != "rpm\\map,20,30,40,50,60,70,80,90,100" {
		t.Errorf("header = %q", lines[0])
	}
	// 16 RPM rows + header.
	if len(lines) != 1+len(g.RPM) {
		t.Fatalf("got %d lines, want %d", len(lines), 1+len(g.RPM))
	}
	// Row 800: the 40-column cell is the trusted 0.922; the rest are 1.000.
	row800 := "800,1.000,1.000,0.922,1.000,1.000,1.000,1.000,1.000,1.000"
	if !slices.Contains(lines, row800) {
		t.Errorf("missing expected row:\n want %q\n in:\n%s", row800, buf.String())
	}
	// Row 1200: the thin 50-column cell must be held at 1.000, not 0.781.
	row1200 := "1200,1.000,1.000,1.000,1.000,1.000,1.000,1.000,1.000,1.000"
	if !slices.Contains(lines, row1200) {
		t.Errorf("thin cell not held at 1.000; row 1200 = %q", findRow(lines, "1200"))
	}
}

func findRow(lines []string, prefix string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix+",") {
			return l
		}
	}
	return ""
}
