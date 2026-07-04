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
	Description string
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

	// Frame must be at least the minimum size for basic sensors
	minRequiredSize := 20
	if len(frame.Data) < minRequiredSize {
		return nil, errors.NewInvalidFrame(
			fmt.Sprintf("frame too small: expected at least %d bytes, got %d", minRequiredSize, len(frame.Data)),
		)
	}

	parsedValues := make(map[string]float64)

	for _, param := range def.Parameters {
		value, err := r.extractParameterValue(frame.Data, &param)
		if err != nil {
			return nil, err
		}
		parsedValues[param.Name] = value
	}

	return &Data{
		EcmDefinition: *def,
		RawData:       frame.Data,
		ParsedValues:  parsedValues,
	}, nil
}

// extractParameterValue reads a parameter's raw bytes from the frame and
// converts them to an engineering value using the parameter's own Factor/Bias
// (or Lookup). There is no per-sensor code path — the conversion lives entirely
// in the ECM definition.
func (r *Registry) extractParameterValue(data []byte, param *Parameter) (float64, error) {
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

// coolantTempLookup converts raw coolant temp byte to Fahrenheit using A033.ads lookup table
func coolantTempLookup(rawValue byte) float64 {
	switch {
	case rawValue <= 12:
		return 302.0
	case rawValue == 13:
		return 293.0
	case rawValue == 14:
		return 284.0
	case rawValue == 15:
		return 275.0
	case rawValue >= 16 && rawValue <= 17:
		return 275.0
	case rawValue >= 18 && rawValue <= 20:
		return 266.0
	case rawValue >= 21 && rawValue <= 22:
		return 257.0
	case rawValue >= 23 && rawValue <= 25:
		return 248.0
	case rawValue >= 26 && rawValue <= 29:
		return 239.0
	case rawValue >= 30 && rawValue <= 33:
		return 230.0
	case rawValue >= 34 && rawValue <= 38:
		return 221.0
	case rawValue >= 39 && rawValue <= 43:
		return 212.0
	case rawValue >= 44 && rawValue <= 49:
		return 203.0
	case rawValue >= 50 && rawValue <= 55:
		return 194.0
	case rawValue >= 56 && rawValue <= 63:
		return 185.0
	case rawValue >= 64 && rawValue <= 71:
		return 176.0
	case rawValue >= 72 && rawValue <= 80:
		return 167.0
	case rawValue >= 81 && rawValue <= 91:
		return 158.0
	case rawValue >= 92 && rawValue <= 101:
		return 149.0
	case rawValue >= 102 && rawValue <= 113:
		return 140.0
	case rawValue >= 114 && rawValue <= 125:
		return 131.0
	case rawValue >= 126 && rawValue <= 138:
		return 122.0
	case rawValue >= 139 && rawValue <= 151:
		return 113.0
	case rawValue >= 152 && rawValue <= 164:
		return 104.0
	case rawValue >= 165 && rawValue <= 176:
		return 95.0
	case rawValue >= 177 && rawValue <= 188:
		return 86.0
	case rawValue >= 189 && rawValue <= 198:
		return 77.0
	case rawValue >= 199 && rawValue <= 208:
		return 68.0
	case rawValue >= 209 && rawValue <= 217:
		return 59.0
	case rawValue >= 218 && rawValue <= 224:
		return 50.0
	case rawValue >= 225 && rawValue <= 230:
		return 41.0
	case rawValue >= 231 && rawValue <= 236:
		return 32.0
	case rawValue >= 237 && rawValue <= 240:
		return 23.0
	case rawValue >= 241 && rawValue <= 244:
		return 14.0
	case rawValue >= 245 && rawValue <= 246:
		return 5.0
	case rawValue >= 247 && rawValue <= 249:
		return -4.0
	case rawValue == 250:
		return -13.0
	case rawValue >= 251 && rawValue <= 252:
		return -22.0
	case rawValue >= 253:
		return -40.0
	}
	return 0.0 // Should never reach here
}
