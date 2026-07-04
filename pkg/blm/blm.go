// Package blm accumulates Block Learn Multiplier (long-term fuel trim) readings
// into an RPM × load grid, so a drive can reveal where the base fuel tune is
// running rich or lean.
//
// BLM interpretation (GM TBI, e.g. 1227747): 128 is neutral — the ECM is
// adding no long-term correction. Below 128 the ECM is *removing* fuel because
// the mixture ran rich, so the base tune has too much fuel in that cell. Above
// 128 it is *adding* fuel because the mixture ran lean. The correction factor
// that drives a cell back toward 128 is simply avg_BLM / 128: multiply the
// base VE/fuel for that cell by it.
//
// BLM is only valid to record in closed loop with block-learn enabled; callers
// must gate on that (see the blm command) before calling Add — this package is
// pure accumulation and does not know about frames or flags.
package blm

// Neutral is the BLM value at which the ECM applies no long-term correction.
const Neutral = 128.0

// DefaultMinSamples is how many readings a cell needs before its average is
// trusted. BLM steps around as the ECM hunts, so one or two samples are noisy;
// WinALDL treats a cell as populated at roughly 3-4. Below this a cell is shown
// as still-accumulating and its correction is held at 1.0 (no change).
const DefaultMinSamples = 4

// DefaultRPM and DefaultMAP are the WinALDL-style display grid: RPM 400..6400
// in 400 steps, MAP 20..100 kPa in 10 steps. Match data/20250601_162123_BLM.txt.
var (
	DefaultRPM = axis(400, 6400, 400)
	DefaultMAP = axis(20, 100, 10)
)

// SparkRPM and SparkMAP are WinALDL's spark-counts display axes: RPM 400..3600
// in 400 steps, MAP 30..100 kPa in 5 steps — a narrower RPM band with finer MAP
// resolution than the fuel-trim grids, to localize knock.
var (
	SparkRPM = axis(400, 3600, 400)
	SparkMAP = axis(30, 100, 5)
)

func axis(lo, hi, step float64) []float64 {
	var a []float64
	for v := lo; v <= hi+1e-9; v += step {
		a = append(a, v)
	}
	return a
}

type cell struct {
	count  int
	sum    float64
	latest float64
}

// Grid bins BLM readings by RPM (rows) and load/MAP (columns). Row and column
// labels are bucket centers; a reading bins to the nearest label on each axis.
type Grid struct {
	RPM   []float64
	MAP   []float64
	cells [][]cell
}

// New builds an empty grid over the given row (RPM) and column (MAP) labels.
func New(rpmLabels, mapLabels []float64) *Grid {
	cells := make([][]cell, len(rpmLabels))
	for i := range cells {
		cells[i] = make([]cell, len(mapLabels))
	}
	return &Grid{RPM: rpmLabels, MAP: mapLabels, cells: cells}
}

// NewDefault builds a grid on the standard WinALDL display axes.
func NewDefault() *Grid { return New(DefaultRPM, DefaultMAP) }

// NewSpark builds a grid on the WinALDL spark-counts display axes.
func NewSpark() *Grid { return New(SparkRPM, SparkMAP) }

// nearest returns the index of the label closest to v (ties go to the lower
// index). Values beyond the ends clamp to the first/last bucket.
func nearest(labels []float64, v float64) int {
	best, bestDist := 0, absf(v-labels[0])
	for i := 1; i < len(labels); i++ {
		if d := absf(v - labels[i]); d < bestDist {
			best, bestDist = i, d
		}
	}
	return best
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Cell returns the row and column indices the given RPM and load bin to.
func (g *Grid) Cell(rpm, load float64) (row, col int) {
	return nearest(g.RPM, rpm), nearest(g.MAP, load)
}

// Add records one BLM reading at the given RPM and load, binning it into the
// nearest cell.
func (g *Grid) Add(rpm, load, blm float64) {
	r, c := g.Cell(rpm, load)
	cell := &g.cells[r][c]
	cell.count++
	cell.sum += blm
	cell.latest = blm
}

// Samples returns the number of readings binned into each cell.
func (g *Grid) Samples() [][]int {
	out := make([][]int, len(g.RPM))
	for r := range out {
		out[r] = make([]int, len(g.MAP))
		for c := range out[r] {
			out[r][c] = g.cells[r][c].count
		}
	}
	return out
}

// Latest returns the most recent BLM in each cell (0 where no data).
func (g *Grid) Latest() [][]float64 {
	return g.floatGrid(func(cl cell) float64 { return cl.latest })
}

// Sum returns the total of all readings binned into each cell (0 where no
// data). For a grid fed per-frame deltas (e.g. knock-counter increments) this
// is the cell's event count, where Average would give the mean delta.
func (g *Grid) Sum() [][]float64 {
	return g.floatGrid(func(cl cell) float64 { return cl.sum })
}

// Average returns the mean BLM in each cell (0 where no data).
func (g *Grid) Average() [][]float64 {
	return g.floatGrid(func(cl cell) float64 {
		if cl.count == 0 {
			return 0
		}
		return cl.sum / float64(cl.count)
	})
}

// Correction returns the fuel-correction factor for each cell: avg_BLM /
// Neutral for cells with data, or 1.0 (no change) for empty cells. Multiply a
// cell's base VE/fuel by this to move the ECM back toward BLM 128.
func (g *Grid) Correction() [][]float64 {
	return g.floatGrid(func(cl cell) float64 {
		if cl.count == 0 {
			return 1.0
		}
		return (cl.sum / float64(cl.count)) / Neutral
	})
}

// CorrectionAtLeast returns the correction grid with cells having fewer than
// minSamples held at 1.0 (no change), so sparse, noisy cells don't suggest a
// fuel edit that only one or two readings support.
func (g *Grid) CorrectionAtLeast(minSamples int) [][]float64 {
	return g.floatGrid(func(cl cell) float64 {
		if cl.count < minSamples {
			return 1.0
		}
		return (cl.sum / float64(cl.count)) / Neutral
	})
}

// PopulatedCells counts cells that have reached minSamples — i.e. cells whose
// average is trustworthy.
func (g *Grid) PopulatedCells(minSamples int) int {
	n := 0
	for r := range g.cells {
		for c := range g.cells[r] {
			if g.cells[r][c].count >= minSamples {
				n++
			}
		}
	}
	return n
}

func (g *Grid) floatGrid(f func(cell) float64) [][]float64 {
	out := make([][]float64, len(g.RPM))
	for r := range out {
		out[r] = make([]float64, len(g.MAP))
		for c := range out[r] {
			out[r][c] = f(g.cells[r][c])
		}
	}
	return out
}

// TotalSamples is the number of readings recorded across all cells.
func (g *Grid) TotalSamples() int {
	n := 0
	for r := range g.cells {
		for c := range g.cells[r] {
			n += g.cells[r][c].count
		}
	}
	return n
}
