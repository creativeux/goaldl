package ecm

import (
	"math"
	"testing"

	"goaldl/pkg/aldl"
)

// TestParseFrameConversions parses a real idle frame and checks the full set of
// conversion kinds: 16-bit (prom), linear factors (rpm, map, battery), a
// lookup table (coolant), and a direct pass-through (speed).
func TestParseFrameConversions(t *testing.T) {
	// Idle fixture frame 0.
	frame := []byte{0x84, 0x18, 0x93, 0x63, 0xA2, 0x00, 0x4A, 0x26, 0x1C, 0x80,
		0xC6, 0x00, 0x00, 0x00, 0x40, 0x89, 0x80, 0xFE, 0x7D, 0x5D}

	r := NewRegistry()
	data, err := r.ParseFrame(&aldl.Frame{Data: frame}, "1227747")
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}

	checks := []struct {
		name string
		want float64
	}{
		{"prom_id", 6291},            // 0x18<<8 | 0x93, Size 2, Factor 1
		{"engine_rpm", 950},          // 0x26=38 * 25
		{"map_voltage", 74 * 0.0196}, // 0x4A=74 * 0.0196
		{"battery_voltage", 13.7},    // 0x89=137 * 0.1
		{"coolant_temp", 104},        // 0xA2=162 via lookup table
		{"vehicle_speed", 0},         // 0x00, Factor 1
	}
	for _, c := range checks {
		if got := data.ParsedValues[c.name]; math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestCoolantTempLookup pins the A033.ads thermistor table: each breakpoint's
// upper-bound raw value must return the table's °F, and the very next raw value
// must step to the next (lower) temperature. Expected values come from the
// A033.ads table, so this guards the hand-transcribed switch.
func TestCoolantTempLookup(t *testing.T) {
	// {upper raw of a range, °F for that range}, ascending.
	table := []struct {
		raw  byte
		temp float64
	}{
		{12, 302}, {13, 293}, {14, 284}, {17, 275}, {20, 266}, {22, 257},
		{25, 248}, {29, 239}, {33, 230}, {38, 221}, {43, 212}, {49, 203},
		{55, 194}, {63, 185}, {71, 176}, {80, 167}, {91, 158}, {101, 149},
		{113, 140}, {125, 131}, {138, 122}, {151, 113}, {164, 104}, {176, 95},
		{188, 86}, {198, 77}, {208, 68}, {217, 59}, {224, 50}, {230, 41},
		{236, 32}, {240, 23}, {244, 14}, {246, 5}, {249, -4}, {250, -13},
		{252, -22}, {255, -40},
	}
	for i, b := range table {
		if got := coolantTempLookup(b.raw); got != b.temp {
			t.Errorf("coolantTempLookup(%d) = %v, want %v", b.raw, got, b.temp)
		}
		// The value one past this breakpoint must belong to the next range.
		if i+1 < len(table) {
			if got := coolantTempLookup(b.raw + 1); got != table[i+1].temp {
				t.Errorf("coolantTempLookup(%d) = %v, want %v (next range)", b.raw+1, got, table[i+1].temp)
			}
		}
	}
	if got := coolantTempLookup(0); got != 302 {
		t.Errorf("coolantTempLookup(0) = %v, want 302", got)
	}
}

// TestExtractParameterValue exercises the generic converter directly, including
// the Bias term (unused by the 1227747 but part of the model) and error cases.
func TestExtractParameterValue(t *testing.T) {
	frame := []byte{0, 100, 0x01, 0x00, 5}

	tests := []struct {
		name    string
		param   Parameter
		want    float64
		wantErr bool
	}{
		{"linear factor+bias", Parameter{Offset: 1, Size: 1, Factor: 0.5, Bias: 10}, 60, false}, // 100*0.5+10
		{"direct", Parameter{Offset: 4, Size: 1, Factor: 1}, 5, false},
		{"16-bit big-endian", Parameter{Offset: 2, Size: 2, Factor: 1}, 256, false}, // 0x0100
		{"lookup overrides factor", Parameter{Offset: 1, Size: 1, Factor: 99, Lookup: func(b byte) float64 { return 7 }}, 7, false},
		{"out of bounds", Parameter{Offset: 4, Size: 2, Factor: 1}, 0, true},
		{"unsupported size", Parameter{Offset: 0, Size: 4, Factor: 1}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractParameterValue(frame, &tt.param)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
