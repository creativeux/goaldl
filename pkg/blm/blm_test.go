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

// idx returns the index of label v in a sorted axis (for readable assertions).
func idx(labels []float64, v float64) int {
	for i, l := range labels {
		if l == v {
			return i
		}
	}
	panic("no such label")
}
