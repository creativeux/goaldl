package ecm

// Fuel-trim extraction for the GM 1227747 (A033). Offsets and MWAF1 flag bits
// per data/A033.ads. Kept here (not in pkg/blm) because it is ECM frame-layout
// knowledge; pkg/blm stays a generic grid accumulator.
const (
	ftOffMAP   = 6  // MAP sensor, raw byte; volts = raw * 0.0196
	ftOffRPM   = 7  // engine speed; RPM = raw * 25
	ftOffMWAF1 = 14 // status word with the closed-loop and BLM-enable bits
	ftOffBLM   = 18 // Block Learn Multiplier

	ftBitBLMEnable  = 1 // MWAF1 bit 1: block learn enabled
	ftBitClosedLoop = 7 // MWAF1 bit 7: loop status (1 = CLOSED)
)

// FramePROM reads the 16-bit PROM ID from a decoded frame (bytes 1-2, big
// endian, GM 1227747 layout), or -1 if the frame is too short.
func FramePROM(frame []byte) int {
	if len(frame) < 3 {
		return -1
	}
	return int(frame[1])<<8 | int(frame[2])
}

// FuelTrim is one frame's fuel-trim state.
type FuelTrim struct {
	RPM        float64
	MapKPa     float64
	BLM        float64
	ClosedLoop bool
	BLMEnabled bool
}

// Recordable reports whether this frame's BLM is valid to record — BLM only
// learns in closed loop with block learn enabled; it is frozen otherwise.
func (ft FuelTrim) Recordable() bool { return ft.ClosedLoop && ft.BLMEnabled }

// FuelTrimSample extracts fuel-trim state from a decoded 1227747 frame. A frame
// shorter than the required layout yields a zero value (Recordable() == false).
func FuelTrimSample(frame []byte) FuelTrim {
	if len(frame) <= ftOffBLM {
		return FuelTrim{}
	}
	mwaf1 := frame[ftOffMWAF1]
	return FuelTrim{
		RPM:        float64(frame[ftOffRPM]) * 25,
		MapKPa:     MapVoltsToKPa(float64(frame[ftOffMAP]) * 0.0196),
		BLM:        float64(frame[ftOffBLM]),
		ClosedLoop: (mwaf1>>ftBitClosedLoop)&1 == 1,
		BLMEnabled: (mwaf1>>ftBitBLMEnable)&1 == 1,
	}
}

// MapVoltsToKPa converts A033 MAP sensor voltage to manifold pressure.
//
// ASSUMPTION — VERIFY against WinALDL: A033.ads reports MAP only in volts, so
// this uses the standard GM 1-bar transfer (~1V≈20 kPa idle vacuum, ~4.9V≈105
// kPa near WOT). If WinALDL's kPa column disagrees, adjust slope/offset here;
// the BLM binning and correction math do not depend on it.
func MapVoltsToKPa(v float64) float64 {
	const slope, offset = 21.25, -1.25
	return slope*v + offset
}
