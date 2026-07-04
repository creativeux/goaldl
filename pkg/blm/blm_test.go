package blm

import (
	"math"
	"strings"
	"testing"
)

func TestBinningNearestLabel(t *testing.T) {
	g := NewDefault() // RPM 400..6400/400, MAP 20..100/10
	// Values bin to the nearest label on each axis.
	g.Add(950, 42, 110)  // -> RPM 800 (950 nearest 800), MAP 40
	g.Add(1250, 58, 120) // -> RPM 1200, MAP 60
	g.Add(1180, 61, 124) // -> RPM 1200, MAP 60 (same cell as previous)

	s := g.Samples()
	if got := s[idx(g.RPM, 800)][idx(g.MAP, 40)]; got != 1 {
		t.Errorf("cell 800/40 samples = %d, want 1", got)
	}
	if got := s[idx(g.RPM, 1200)][idx(g.MAP, 60)]; got != 2 {
		t.Errorf("cell 1200/60 samples = %d, want 2", got)
	}
	if g.TotalSamples() != 3 {
		t.Errorf("TotalSamples = %d, want 3", g.TotalSamples())
	}
}

// Correction math is anchored to data/20250601_162123_BLM.txt: a cell
// averaging BLM 109.2 has correction 0.853 there.
func TestCorrectionMatchesReference(t *testing.T) {
	g := NewDefault()
	// Two readings averaging 109.2 in one cell.
	g.Add(400, 40, 108.4)
	g.Add(400, 40, 110.0)
	avg := g.Average()[idx(g.RPM, 400)][idx(g.MAP, 40)]
	if math.Abs(avg-109.2) > 1e-9 {
		t.Fatalf("average = %.4f, want 109.2", avg)
	}
	corr := g.Correction()[idx(g.RPM, 400)][idx(g.MAP, 40)]
	if math.Abs(corr-0.853) > 0.0005 {
		t.Errorf("correction = %.4f, want ~0.853 (109.2/128)", corr)
	}
}

func TestEmptyCellsAreNeutral(t *testing.T) {
	g := NewDefault()
	g.Add(2000, 50, 120) // populate one cell only
	corr := g.Correction()
	// An untouched cell must read 1.0 (no change), not 0.
	r, c := idx(g.RPM, 4000), idx(g.MAP, 90)
	if corr[r][c] != 1.0 {
		t.Errorf("empty cell correction = %v, want 1.0", corr[r][c])
	}
	if g.Latest()[r][c] != 0 {
		t.Errorf("empty cell latest = %v, want 0", g.Latest()[r][c])
	}
}

func TestLatestVsAverage(t *testing.T) {
	g := NewDefault()
	g.Add(2000, 50, 120)
	g.Add(2000, 50, 130) // latest
	r, c := idx(g.RPM, 2000), idx(g.MAP, 50)
	if got := g.Latest()[r][c]; got != 130 {
		t.Errorf("latest = %v, want 130", got)
	}
	if got := g.Average()[r][c]; got != 125 {
		t.Errorf("average = %v, want 125", got)
	}
}

func TestClampsOutOfRange(t *testing.T) {
	g := NewDefault()
	g.Add(9000, 5, 118) // RPM above top (->6400), MAP below bottom (->20)
	if g.Samples()[idx(g.RPM, 6400)][idx(g.MAP, 20)] != 1 {
		t.Error("out-of-range reading did not clamp to the corner cell")
	}
}

func TestCorrectionAtLeast(t *testing.T) {
	g := NewDefault()
	// Cell A: 4 samples averaging 116 -> trusted, correction 116/128 = 0.90625.
	for i := 0; i < 4; i++ {
		g.Add(1600, 40, 116)
	}
	// Cell B: 2 samples -> below default threshold -> held at 1.0.
	g.Add(2000, 50, 100)
	g.Add(2000, 50, 100)

	corr := g.CorrectionAtLeast(4)
	if got := corr[idx(g.RPM, 1600)][idx(g.MAP, 40)]; math.Abs(got-0.90625) > 1e-9 {
		t.Errorf("trusted cell correction = %.5f, want 0.90625", got)
	}
	if got := corr[idx(g.RPM, 2000)][idx(g.MAP, 50)]; got != 1.0 {
		t.Errorf("below-threshold cell correction = %v, want 1.0 (held)", got)
	}
	// A lower threshold trusts cell B and applies its correction. Expected
	// value is the independently-computed literal 100/128 = 0.78125, not the
	// production formula re-run, so a broken divisor would be caught.
	corr3 := g.CorrectionAtLeast(2)
	if got := corr3[idx(g.RPM, 2000)][idx(g.MAP, 50)]; math.Abs(got-0.78125) > 1e-9 {
		t.Errorf("cell B at min=2 correction = %.5f, want 0.78125", got)
	}
}

func TestPopulatedCells(t *testing.T) {
	g := NewDefault()
	for i := 0; i < 4; i++ {
		g.Add(1600, 40, 116) // reaches 4
	}
	g.Add(2000, 50, 120) // only 1
	g.Add(2000, 50, 120) // 2
	g.Add(2000, 50, 120) // 3

	if got := g.PopulatedCells(4); got != 1 {
		t.Errorf("PopulatedCells(4) = %d, want 1", got)
	}
	if got := g.PopulatedCells(3); got != 2 {
		t.Errorf("PopulatedCells(3) = %d, want 2", got)
	}
	if got := g.PopulatedCells(1); got != 2 {
		t.Errorf("PopulatedCells(1) = %d, want 2 (distinct cells touched)", got)
	}
}

func TestRenderInt(t *testing.T) {
	g := NewDefault()
	g.Add(800, 40, 110)
	g.Add(800, 40, 112)
	out := g.RenderInt("Samples", g.Samples())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "Samples" {
		t.Errorf("title = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "RPM \\ MAP\t20\t30") {
		t.Errorf("header = %q", lines[1])
	}
	// The 800/40 cell shows 2; find that row and check the 40-column value.
	var row800 string
	for _, l := range lines {
		if strings.HasPrefix(l, "800\t") {
			row800 = l
		}
	}
	if fields := strings.Split(row800, "\t"); fields[3] != "2" { // rpm,20,30,40 -> index 3
		t.Errorf("800/40 sample count = %q, want 2 (row %q)", fields[3], row800)
	}
}

func TestRenderShape(t *testing.T) {
	g := NewDefault()
	g.Add(800, 40, 110)
	out := g.RenderFloat("Average", g.Average(), 1)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "Average" {
		t.Errorf("first line = %q, want title", lines[0])
	}
	if !strings.HasPrefix(lines[1], "RPM \\ MAP\t20\t30") {
		t.Errorf("header = %q", lines[1])
	}
	// title + header + 16 RPM rows
	if len(lines) != 2+len(g.RPM) {
		t.Errorf("got %d lines, want %d", len(lines), 2+len(g.RPM))
	}
}

func TestSum(t *testing.T) {
	g := NewDefault()
	g.Add(1600, 40, 2) // two deltas into one cell: sum 5, samples 2
	g.Add(1600, 40, 3)
	r, c := idx(g.RPM, 1600), idx(g.MAP, 40)
	if got := g.Sum()[r][c]; got != 5 {
		t.Errorf("sum = %v, want 5", got)
	}
	if got := g.Samples()[r][c]; got != 2 {
		t.Errorf("samples = %d, want 2", got)
	}
	if got := g.Sum()[idx(g.RPM, 4000)][idx(g.MAP, 90)]; got != 0 {
		t.Errorf("empty cell sum = %v, want 0", got)
	}
}

func TestNewSparkAxes(t *testing.T) {
	g := NewSpark()
	// WinALDL spark display grid: RPM 400..3600/400, MAP 30..100/5.
	if len(g.RPM) != 9 || g.RPM[0] != 400 || g.RPM[8] != 3600 || g.RPM[1]-g.RPM[0] != 400 {
		t.Errorf("spark RPM axis = %v, want 400..3600 step 400", g.RPM)
	}
	if len(g.MAP) != 15 || g.MAP[0] != 30 || g.MAP[14] != 100 || g.MAP[1]-g.MAP[0] != 5 {
		t.Errorf("spark MAP axis = %v, want 30..100 step 5", g.MAP)
	}
}

// idx returns the index of label v in a sorted axis (for readable assertions).
func idx(labels []float64, v float64) int {
	for i, l := range labels {
		if l == v {
			return i
		}
	}
	panic("no such label")
}
