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

// TestExtractParameterValue exercises the generic converter directly, including
// the Bias term (unused by the 1227747 but part of the model) and error cases.
func TestExtractParameterValue(t *testing.T) {
	r := NewRegistry()
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
			got, err := r.extractParameterValue(frame, &tt.param)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
