package decoder

// Encoder generates the raw UART byte stream a capture would contain for
// known frame data — the inverse of Decoder. Used to validate the decode
// logic offline and to produce demo captures (goaldl simulate).
type Encoder struct {
	cfg Config
	// Pulse widths to synthesize, in microseconds. Real hardware jitters;
	// tests vary these across their valid ranges.
	ShortPulseMicros float64 // logic 0 (GM 1227747: ~365μs)
	LongPulseMicros  float64 // logic 1 (GM 1227747: ~1875μs)
}

// NewEncoder returns an encoder with the GM 1227747's nominal pulse widths.
func NewEncoder(cfg Config) *Encoder {
	return &Encoder{cfg: cfg, ShortPulseMicros: 365, LongPulseMicros: 1875}
}

// EncodeBit returns the UART byte one ALDL bit produces at the configured
// baud rate: the pulse covers the start bit plus k data bits, leaving the
// byte value 0xFF<<k (then inverted if the config is inverted).
func (e *Encoder) EncodeBit(bit byte) byte {
	pulse := e.ShortPulseMicros
	if bit == 1 {
		pulse = e.LongPulseMicros
	}
	k := int(pulse/e.cfg.bitMicros()) - 1 // data bits covered after the start bit
	k = min(max(k, 0), 8)                 // >8 would spill into a second character
	b := byte(0xFF << k)
	if e.cfg.Invert {
		b = ^b
	}
	return b
}

// EncodeFrame returns the byte stream for one sync character followed by the
// frame's data characters (mode bit 0 + 8 data bits MSB first).
func (e *Encoder) EncodeFrame(data []byte) []byte {
	out := make([]byte, 0, e.cfg.SyncBits+len(data)*9)
	for i := 0; i < e.cfg.SyncBits; i++ {
		out = append(out, e.EncodeBit(1))
	}
	for _, d := range data {
		out = append(out, e.EncodeBit(0)) // mode bit
		for i := 7; i >= 0; i-- {
			out = append(out, e.EncodeBit((d>>i)&1))
		}
	}
	return out
}

// EncodeStream concatenates multiple frames into one capture.
func (e *Encoder) EncodeStream(frames [][]byte) []byte {
	var out []byte
	for _, f := range frames {
		out = append(out, e.EncodeFrame(f)...)
	}
	return out
}
