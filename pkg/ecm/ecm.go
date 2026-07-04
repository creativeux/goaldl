package ecm

import (
	"fmt"

	"goaldl/pkg/aldl"
	"goaldl/pkg/errors"
)

// Definition represents an ECM configuration
type Definition struct {
	PartNumber   string
	Description  string
	DataStreamID byte
	FrameSize    int
	Parameters   []Parameter
	FlagWords    []FlagWord  // status words with labeled bits (see flags.go)
	ErrorCodes   []ErrorCode // malfunction-flag bits → GM trouble codes
	ByteLabels   []string    // per-byte frame labels (WinALDL naming), len == FrameSize
}

// Lookup converts a raw byte to an engineering value through a non-linear
// table (e.g. a thermistor curve).
type Lookup func(raw byte) float64

// Parameter describes one sensor in an ECM's data frame. Its engineering value
// is computed from the raw bytes at Offset as either a linear transform
// (raw*Factor + Bias) or, when Lookup is set, a table lookup — mirroring the
// dFactor/dOffset/iLookupTableIndex fields of an A033.ads definition. The
// conversion is data, not code: adding a sensor or changing a scale never
// touches the parser.
type Parameter struct {
	Name        string
	Offset      int // byte position in the frame
	Size        int // number of bytes (1, or 2 for a 16-bit big-endian value)
	Unit        string
	Factor      float64 // linear scale applied to the raw value
	Bias        float64 // additive offset applied after Factor
	Lookup      Lookup  // non-linear conversion; when set, Factor/Bias are ignored
	Alt         *AltConversion
	Description string
}

// AltConversion is a second engineering conversion of the same raw byte, for
// dual-unit display (WinALDL shows US and metric side by side: °F/°C, MPH/KPH,
// MAP V/kPa, TPS V/%). Same shape as the primary conversion — pure data.
// Applies to single-byte parameters only.
type AltConversion struct {
	Factor float64
	Bias   float64
	Lookup Lookup // when set, Factor/Bias are ignored
	Unit   string
}

// Default TPS calibration (WinALDL's defaults): throttle-position percent is
// (volts−V0)/(V100−V0)×100. Overridable per vehicle via WithTPSCalibration.
const (
	DefaultTPS0   = 0.54 // volts at 0% throttle
	DefaultTPS100 = 4.60 // volts at 100% throttle
)

// TPSPercentAlt builds the TPS percent alternate conversion for a calibration.
// Verified against the WinALDL log with the defaults (raw 28→0.2%, 39→5.5%,
// 62→16.6%).
func TPSPercentAlt(v0, v100 float64) *AltConversion {
	span := v100 - v0
	return &AltConversion{Factor: 0.0196 * 100 / span, Bias: -100 * v0 / span, Unit: "%"}
}

// WithTPSCalibration returns a copy of the definition whose tps_voltage
// parameter converts to percent using the given calibration voltages. A
// degenerate calibration (v100 ≤ v0) returns the definition unchanged —
// callers validate and warn. The receiver is never modified (Parameters are
// cloned), so registry-held definitions stay pristine.
func (d *Definition) WithTPSCalibration(v0, v100 float64) *Definition {
	if v100 <= v0 {
		return d
	}
	out := *d
	out.Parameters = make([]Parameter, len(d.Parameters))
	copy(out.Parameters, d.Parameters)
	for i := range out.Parameters {
		if out.Parameters[i].Name == "tps_voltage" {
			out.Parameters[i].Alt = TPSPercentAlt(v0, v100)
		}
	}
	return &out
}

// Data represents parsed ECM data
type Data struct {
	EcmDefinition Definition
	RawData       []byte
	ParsedValues  map[string]float64
}

// Registry manages ECM definitions
type Registry struct {
	definitions map[string]Definition
}

// NewRegistry creates a new ECM registry with all available definitions
func NewRegistry() *Registry {
	r := &Registry{
		definitions: make(map[string]Definition),
	}

	// Load all ECM definitions
	for _, def := range getAllDefinitions() {
		r.definitions[def.PartNumber] = def
	}

	return r
}

// GetDefinition returns an ECM definition by part number
func (r *Registry) GetDefinition(partNumber string) (*Definition, bool) {
	def, ok := r.definitions[partNumber]
	return &def, ok
}

// ListSupportedECMs returns a list of supported ECM part numbers
func (r *Registry) ListSupportedECMs() []string {
	ecms := make([]string, 0, len(r.definitions))
	for pn := range r.definitions {
		ecms = append(ecms, pn)
	}
	return ecms
}

// ParseFrame parses an ALDL frame using the specified ECM definition
func (r *Registry) ParseFrame(frame *aldl.Frame, ecmPart string) (*Data, error) {
	def, ok := r.GetDefinition(ecmPart)
	if !ok {
		return nil, errors.NewUnsupportedECM(ecmPart)
	}
	parsedValues, err := def.Parse(frame.Data)
	if err != nil {
		return nil, err
	}
	return &Data{
		EcmDefinition: *def,
		RawData:       frame.Data,
		ParsedValues:  parsedValues,
	}, nil
}

// Parse converts a raw frame to engineering values, one entry per parameter.
// It is the definition-level core of Registry.ParseFrame, usable directly by
// consumers that hold a (possibly calibrated) Definition.
func (d *Definition) Parse(data []byte) (map[string]float64, error) {
	// Frame must be at least the minimum size for basic sensors
	minRequiredSize := 20
	if len(data) < minRequiredSize {
		return nil, errors.NewInvalidFrame(
			fmt.Sprintf("frame too small: expected at least %d bytes, got %d", minRequiredSize, len(data)),
		)
	}
	parsedValues := make(map[string]float64)
	for _, param := range d.Parameters {
		value, err := extractParameterValue(data, &param)
		if err != nil {
			return nil, err
		}
		parsedValues[param.Name] = value
	}
	return parsedValues, nil
}

// extractParameterValue reads a parameter's raw bytes from the frame and
// converts them to an engineering value using the parameter's own Factor/Bias
// (or Lookup). There is no per-sensor code path — the conversion lives entirely
// in the ECM definition.
func extractParameterValue(data []byte, param *Parameter) (float64, error) {
	if param.Offset < 0 || param.Offset+param.Size > len(data) {
		return 0, errors.NewInvalidFrame(
			fmt.Sprintf("parameter %s exceeds frame bounds", param.Name),
		)
	}

	var raw float64
	switch param.Size {
	case 1:
		raw = float64(data[param.Offset])
	case 2:
		raw = float64(uint16(data[param.Offset])<<8 | uint16(data[param.Offset+1]))
	default:
		return 0, errors.WrapProtocol(nil,
			fmt.Sprintf("unsupported parameter size: %d", param.Size),
		)
	}

	if param.Lookup != nil {
		return param.Lookup(byte(raw)), nil
	}
	return raw*param.Factor + param.Bias, nil
}

// coolantTempTable is the GM 1227747 (A033.ads) coolant thermistor curve as
// data: each entry is the inclusive upper raw count of a range and the °F it
// maps to, ascending. The °F column steps by a constant 9° (except the final
// entry) but the raw ranges widen toward the middle — thermistor nonlinearity —
// so the mapping can't be reduced to a formula and is expressed as a table.
var coolantTempTable = []struct {
	maxRaw byte
	degF   float64
}{
	{12, 302}, {13, 293}, {14, 284}, {17, 275}, {20, 266}, {22, 257},
	{25, 248}, {29, 239}, {33, 230}, {38, 221}, {43, 212}, {49, 203},
	{55, 194}, {63, 185}, {71, 176}, {80, 167}, {91, 158}, {101, 149},
	{113, 140}, {125, 131}, {138, 122}, {151, 113}, {164, 104}, {176, 95},
	{188, 86}, {198, 77}, {208, 68}, {217, 59}, {224, 50}, {230, 41},
	{236, 32}, {240, 23}, {244, 14}, {246, 5}, {249, -4}, {250, -13},
	{252, -22}, {255, -40},
}

// coolantTempLookup returns the °F for a raw coolant byte: the value of the
// first range whose upper bound the raw count does not exceed. The final entry
// covers 255, so a valid byte always matches.
func coolantTempLookup(rawValue byte) float64 {
	for _, e := range coolantTempTable {
		if rawValue <= e.maxRaw {
			return e.degF
		}
	}
	return coolantTempTable[len(coolantTempTable)-1].degF
}
