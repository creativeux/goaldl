package ecm

import (
	"math"
	"testing"
)

func def1227747(t *testing.T) *Definition {
	t.Helper()
	def, ok := NewRegistry().GetDefinition("1227747")
	if !ok {
		t.Fatal("no 1227747 definition")
	}
	return def
}

// frameWith returns a 20-byte frame with the given byte set.
func frameWith(offset int, value byte) []byte {
	f := make([]byte, 20)
	f[offset] = value
	return f
}

func findWord(t *testing.T, flags []FlagWordStatus, word string) FlagWordStatus {
	t.Helper()
	for _, w := range flags {
		if w.Word == word {
			return w
		}
	}
	t.Fatalf("no flag word %q", word)
	return FlagWordStatus{}
}

func setBits(w FlagWordStatus) []string {
	var names []string
	for _, b := range w.Bits {
		if b.Set {
			names = append(names, b.Name)
		}
	}
	return names
}

// TestDecodeFlagsLogOracle decodes flag bytes taken from real rows of the
// WinALDL ground-truth log (data/20250601_111156_LOG.txt), whose per-bit
// columns are the oracle: MW2=128 sets exactly the Idle flag, MWAF1=64 exactly
// the Rich flag, MCU2IO=128 exactly No-A/C-requested.
func TestDecodeFlagsLogOracle(t *testing.T) {
	def := def1227747(t)

	tests := []struct {
		word    string
		offset  int
		raw     byte
		wantSet []string
	}{
		{"MW2", 0, 128, []string{"Idle flag"}},
		{"MWAF1", 14, 64, []string{"Rich/lean"}},
		{"MCU2IO", 16, 128, []string{"No A/C requested"}},
		{"MWAF1", 14, 0, nil},
		// Closed loop + block learn enabled (0x82) — the recordable state.
		{"MWAF1", 14, 0x82, []string{"Block learn enable", "Loop status"}},
	}
	for _, tt := range tests {
		flags := DecodeFlags(def, frameWith(tt.offset, tt.raw))
		w := findWord(t, flags, tt.word)
		if w.Raw != tt.raw {
			t.Errorf("%s raw = 0x%02X, want 0x%02X", tt.word, w.Raw, tt.raw)
		}
		got := setBits(w)
		if len(got) != len(tt.wantSet) {
			t.Errorf("%s=0x%02X set bits = %v, want %v", tt.word, tt.raw, got, tt.wantSet)
			continue
		}
		for i := range got {
			if got[i] != tt.wantSet[i] {
				t.Errorf("%s=0x%02X set bits = %v, want %v", tt.word, tt.raw, got, tt.wantSet)
			}
		}
	}
}

// TestDecodeFlagsShortFrame: a frame shorter than the frame size yields nil
// ("no flag data yet"), never a partial decode.
func TestDecodeFlagsShortFrame(t *testing.T) {
	def := def1227747(t)
	if got := DecodeFlags(def, make([]byte, 10)); got != nil {
		t.Errorf("short frame decoded to %v, want nil", got)
	}
	if got := DecodeFlags(nil, make([]byte, 20)); got != nil {
		t.Errorf("nil definition decoded to %v, want nil", got)
	}
}

// TestFuelTrimFlagTableConsistency guards the two encodings of the same ECM
// fact: the fast-path bit constants in fueltrim.go and the MWAF1 flag table
// must agree on which bits mean closed-loop and BLM-enable.
func TestFuelTrimFlagTableConsistency(t *testing.T) {
	def := def1227747(t)

	for _, tt := range []struct {
		mwaf1    byte
		bitName  string
		ftAssert func(FuelTrim) bool
	}{
		{1 << ftBitClosedLoop, "Loop status", func(ft FuelTrim) bool { return ft.ClosedLoop }},
		{1 << ftBitBLMEnable, "Block learn enable", func(ft FuelTrim) bool { return ft.BLMEnabled }},
	} {
		f := frameWith(14, tt.mwaf1)
		if !tt.ftAssert(FuelTrimSample(f)) {
			t.Errorf("FuelTrimSample(MWAF1=0x%02X) did not set the expected field", tt.mwaf1)
		}
		w := findWord(t, DecodeFlags(def, f), "MWAF1")
		got := setBits(w)
		if len(got) != 1 || got[0] != tt.bitName {
			t.Errorf("flag table for MWAF1=0x%02X sets %v, want exactly [%s]", tt.mwaf1, got, tt.bitName)
		}
	}
}

// TestDecodeCodes checks single-bit → single-code mapping for each MALFFLG
// byte (A033.ads is the oracle), the sorted order, and the multi-bit case.
func TestDecodeCodes(t *testing.T) {
	def := def1227747(t)

	single := []struct {
		offset int
		raw    byte
		code   int
	}{
		{11, 1 << 7, 12}, // no reference pulses
		{11, 1 << 0, 24}, // VSS
		{12, 1 << 5, 32}, // EGR
		{12, 1 << 0, 42}, // EST monitor
		{13, 1 << 7, 43}, // ESC knock
		{13, 1 << 6, 44}, // O2 lean
		{13, 1 << 0, 55}, // A/D unit
	}
	for _, tt := range single {
		codes := DecodeCodes(def, frameWith(tt.offset, tt.raw))
		var set []int
		for _, c := range codes {
			if c.Set {
				set = append(set, c.Code)
			}
		}
		if len(set) != 1 || set[0] != tt.code {
			t.Errorf("byte %d = 0x%02X sets codes %v, want [%d]", tt.offset, tt.raw, set, tt.code)
		}
	}

	// No malfunction bytes set → no codes set, full list present and sorted.
	codes := DecodeCodes(def, make([]byte, 20))
	if len(codes) != 24 {
		t.Fatalf("decoded %d codes, want 24", len(codes))
	}
	for i, c := range codes {
		if c.Set {
			t.Errorf("code %d set on a clean frame", c.Code)
		}
		if i > 0 && codes[i-1].Code >= c.Code {
			t.Errorf("codes not sorted: %d before %d", codes[i-1].Code, c.Code)
		}
	}
	if codes[0].Word != "MALFFLG1" {
		t.Errorf("code 12 word = %q, want MALFFLG1", codes[0].Word)
	}

	// Multiple codes across bytes decode together.
	f := make([]byte, 20)
	f[11], f[13] = 1<<6, 1<<5 // 13 O2 open + 45 O2 rich
	var set []int
	for _, c := range DecodeCodes(def, f) {
		if c.Set {
			set = append(set, c.Code)
		}
	}
	if len(set) != 2 || set[0] != 13 || set[1] != 45 {
		t.Errorf("multi-bit frame sets %v, want [13 45]", set)
	}
}

// TestMapVoltsToKPaLogOracle pins the WinALDL-verified MAP transfer to raw
// byte / kPa pairs read straight out of the ground-truth log.
func TestMapVoltsToKPaLogOracle(t *testing.T) {
	for _, tt := range []struct {
		raw byte
		kpa float64
	}{
		{49, 28.4}, {101, 47.6}, {183, 77.9},
	} {
		got := MapVoltsToKPa(float64(tt.raw) * 0.0196)
		if math.Abs(got-tt.kpa) > 0.05 {
			t.Errorf("MapVoltsToKPa(raw %d) = %.2f kPa, want %.1f (WinALDL log)", tt.raw, got, tt.kpa)
		}
	}
}

// TestTPSPercentLogOracle pins the TPS percent conversion (default 0.54/4.60V
// calibration) to raw/percent pairs from the ground-truth log.
func TestTPSPercentLogOracle(t *testing.T) {
	alt := TPSPercentAlt(DefaultTPS0, DefaultTPS100)
	for _, tt := range []struct {
		raw byte
		pct float64
	}{
		{28, 0.2}, {39, 5.5}, {62, 16.6},
	} {
		got := float64(tt.raw)*alt.Factor + alt.Bias
		if math.Abs(got-tt.pct) > 0.05 {
			t.Errorf("TPS%% (raw %d) = %.2f, want %.1f (WinALDL log)", tt.raw, got, tt.pct)
		}
	}
}

// TestWithTPSCalibration: a custom calibration rescales the tps Alt on a copy,
// leaves the original untouched, and a degenerate range is a no-op.
func TestWithTPSCalibration(t *testing.T) {
	def := def1227747(t)
	tpsAlt := func(d *Definition) *AltConversion {
		for _, p := range d.Parameters {
			if p.Name == "tps_voltage" {
				return p.Alt
			}
		}
		t.Fatal("no tps_voltage parameter")
		return nil
	}
	orig := tpsAlt(def)

	cal := def.WithTPSCalibration(1.0, 4.0)
	// raw such that volts=1.0 → 0%; volts=4.0 → 100%.
	if got := (1.0/0.0196)*tpsAlt(cal).Factor + tpsAlt(cal).Bias; math.Abs(got) > 1e-6 {
		t.Errorf("calibrated 1.0V = %.3f%%, want 0", got)
	}
	if got := (4.0/0.0196)*tpsAlt(cal).Factor + tpsAlt(cal).Bias; math.Abs(got-100) > 1e-6 {
		t.Errorf("calibrated 4.0V = %.3f%%, want 100", got)
	}
	if tpsAlt(def) != orig {
		t.Error("WithTPSCalibration mutated the source definition")
	}
	if got := def.WithTPSCalibration(4.0, 1.0); got != def {
		t.Error("degenerate calibration should return the definition unchanged")
	}
}
