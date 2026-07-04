package ecm

import (
	"math"
	"testing"
)

// closedLoopFrame builds a 20-byte frame with the given RPM/MAP/BLM bytes and
// MWAF1 set for closed loop + block learn enabled (bits 7 and 1 = 0x82).
func closedLoopFrame(rpmByte, mapByte, blmByte, mwaf1 byte) []byte {
	f := make([]byte, 20)
	f[6] = mapByte
	f[7] = rpmByte
	f[14] = mwaf1
	f[18] = blmByte
	return f
}

func TestFuelTrimSample(t *testing.T) {
	f := closedLoopFrame(64, 99, 118, 0x82) // RPM 64*25=1600, MWAF1 bits 7&1
	ft := FuelTrimSample(f)

	if ft.RPM != 1600 {
		t.Errorf("RPM = %v, want 1600", ft.RPM)
	}
	if ft.BLM != 118 {
		t.Errorf("BLM = %v, want 118", ft.BLM)
	}
	// MAP: kPa = (99+28.06)/2.71 ≈ 46.89 (WinALDL-verified transfer).
	if math.Abs(ft.MapKPa-46.89) > 0.1 {
		t.Errorf("MapKPa = %.2f, want ~46.89", ft.MapKPa)
	}
	if !ft.ClosedLoop || !ft.BLMEnabled || !ft.Recordable() {
		t.Errorf("flags: closed=%v blm=%v recordable=%v, want all true", ft.ClosedLoop, ft.BLMEnabled, ft.Recordable())
	}
}

func TestFuelTrimGating(t *testing.T) {
	// Open loop (bit 7 clear), BLM enabled (bit 1 set) = 0x02.
	if FuelTrimSample(closedLoopFrame(64, 99, 118, 0x02)).Recordable() {
		t.Error("open-loop frame should not be recordable")
	}
	// Closed loop (bit 7 set), BLM disabled (bit 1 clear) = 0x80.
	if FuelTrimSample(closedLoopFrame(64, 99, 118, 0x80)).Recordable() {
		t.Error("block-learn-disabled frame should not be recordable")
	}
	// Neither set.
	if FuelTrimSample(closedLoopFrame(64, 99, 118, 0x00)).Recordable() {
		t.Error("no-flags frame should not be recordable")
	}
}

func TestFuelTrimShortFrame(t *testing.T) {
	if FuelTrimSample(make([]byte, 10)).Recordable() {
		t.Error("short frame should yield a non-recordable zero value")
	}
}
