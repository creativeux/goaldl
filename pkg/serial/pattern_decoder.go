package serial

import (
	"fmt"
	"goaldl/pkg/errors"
)

// PatternDecoder tries to decode ALDL by pattern matching rather than sync detection
type PatternDecoder struct {
	serial   *AldlSerial
	baudRate int
	Debug    bool
}

// NewPatternDecoder creates a decoder that uses pattern matching
func NewPatternDecoder(portName string, baudRate int) (*PatternDecoder, error) {
	ser, err := NewWithBaudRate(portName, baudRate)
	if err != nil {
		return nil, err
	}

	return &PatternDecoder{
		serial:   ser,
		baudRate: baudRate,
	}, nil
}

// Close closes the serial port
func (d *PatternDecoder) Close() error {
	return d.serial.Close()
}

// ReadRawBytes reads a chunk of raw UART bytes
func (d *PatternDecoder) ReadRawBytes(count int) ([]byte, error) {
	buffer := make([]byte, count)

	for i := 0; i < count; i++ {
		b, err := d.serial.ReadByte()
		if err != nil {
			return buffer[:i], err
		}
		buffer[i] = b
	}

	return buffer, nil
}

// BytesToBits converts UART bytes to a bit stream (MSB first)
func (d *PatternDecoder) BytesToBits(bytes []byte) []byte {
	bits := make([]byte, len(bytes)*8)

	for i, b := range bytes {
		for j := 7; j >= 0; j-- {
			if (b & (1 << j)) != 0 {
				bits[i*8+(7-j)] = 1
			} else {
				bits[i*8+(7-j)] = 0
			}
		}
	}

	return bits
}

// DecodeAldlBitFromPulse uses pulse width measurement to decode a single ALDL bit
// Counts consecutive LOW bits (zeros) starting from position
func (d *PatternDecoder) DecodeAldlBitFromPulse(bits []byte, pos int) (byte, int, error) {
	if pos >= len(bits) {
		return 0, pos, errors.NewInvalidFrame("position out of bounds")
	}

	// Skip any HIGH bits (idle state)
	for pos < len(bits) && bits[pos] == 1 {
		pos++
	}

	if pos >= len(bits) {
		return 0, pos, errors.NewInvalidFrame("reached end while skipping idle")
	}

	// Now we're at start of a LOW pulse, count how many LOW bits
	lowCount := 0
	for pos < len(bits) && bits[pos] == 0 {
		lowCount++
		pos++
	}

	// At 4800 baud: 1 bit = 208 μs
	// Logic 0: 360-370 μs = ~1.7-1.8 bits → expect 1-3 low bits
	// Logic 1: 1850-4400 μs = ~9-21 bits → expect 7-25 low bits

	if lowCount >= 1 && lowCount <= 5 {
		return 0, pos, nil // Logic 0
	} else if lowCount >= 6 {
		return 1, pos, nil // Logic 1
	}

	return 0, pos, errors.NewInvalidFrame(fmt.Sprintf("ambiguous pulse: %d low bits", lowCount))
}

// DecodeAldlByteFromBits decodes a 9-bit ALDL byte from bit stream
// Returns the 8 data bits (mode bit is read but discarded)
func (d *PatternDecoder) DecodeAldlByteFromBits(bits []byte, pos int) (byte, int, error) {
	var result byte

	// Read 9 bits total (1 mode + 8 data)
	// We'll just read all 9 and use the last 8 as the data
	for bitNum := 0; bitNum < 9; bitNum++ {
		bit, newPos, err := d.DecodeAldlBitFromPulse(bits, pos)
		if err != nil {
			return 0, newPos, err
		}
		pos = newPos

		// Skip mode bit (first bit), only accumulate data bits
		if bitNum > 0 {
			if bit == 1 {
				result |= (1 << (8 - bitNum))
			}
		}
	}

	return result, pos, nil
}

// FindPromIdPattern searches for PROM ID [24, 147] in decoded byte stream
// Returns the byte offset where found, or -1 if not found
func (d *PatternDecoder) FindPromIdPattern(bits []byte) (int, []byte, error) {
	if d.Debug {
		fmt.Printf("Searching for PROM ID pattern in %d bits...\n", len(bits))
	}

	// Try to decode bytes starting from different bit positions
	// This accounts for unknown bit alignment
	for startBit := 0; startBit < 8*20 && startBit < len(bits)/2; startBit++ {
		if d.Debug && startBit%8 == 0 {
			fmt.Printf("  Trying start offset %d bits...\n", startBit)
		}

		// Try to decode 50 bytes from this starting position
		pos := startBit
		decoded := make([]byte, 0, 50)

		for len(decoded) < 50 && pos < len(bits)-200 {
			b, newPos, err := d.DecodeAldlByteFromBits(bits, pos)
			if err != nil {
				break
			}
			decoded = append(decoded, b)
			pos = newPos
		}

		if len(decoded) < 20 {
			continue
		}

		// Debug: show first few decoded bytes
		if d.Debug && len(decoded) >= 10 {
			fmt.Printf("    Decoded %d bytes, first 10: ", len(decoded))
			for i := 0; i < 10; i++ {
				fmt.Printf("%d ", decoded[i])
			}
			fmt.Println()
		}

		// Search for PROM ID [24, 147] at bytes 1-2 in this decoded stream
		for i := 0; i < len(decoded)-20; i++ {
			if decoded[i+1] == 24 && decoded[i+2] == 147 {
				if d.Debug {
					fmt.Printf("✓ Found PROM ID at start bit %d, byte offset %d\n", startBit, i)
					fmt.Printf("  Frame: MW2=%d PROMIDA=%d PROMIDB=%d\n", decoded[i], decoded[i+1], decoded[i+2])
					fmt.Print("  First 10 bytes: ")
					for j := 0; j < 10 && i+j < len(decoded); j++ {
						fmt.Printf("%d ", decoded[i+j])
					}
					fmt.Println()
				}

				// Return the 20-byte frame
				frame := make([]byte, 20)
				copy(frame, decoded[i:i+20])
				return startBit + i*9, frame, nil // Return bit position and frame
			}
		}
	}

	return -1, nil, errors.NewInvalidFrame("PROM ID pattern not found")
}

// ReadFrameByPattern reads raw bytes, searches for PROM ID pattern
func (d *PatternDecoder) ReadFrameByPattern(rawByteCount int) ([]byte, error) {
	// Read a large buffer of raw UART bytes
	rawBytes, err := d.ReadRawBytes(rawByteCount)
	if err != nil {
		return nil, err
	}

	if d.Debug {
		fmt.Printf("Read %d raw UART bytes\n", len(rawBytes))
		fmt.Print("First 20 bytes: ")
		for i := 0; i < 20 && i < len(rawBytes); i++ {
			fmt.Printf("%02X ", rawBytes[i])
		}
		fmt.Println()
	}

	// Convert to bit stream
	bits := d.BytesToBits(rawBytes)

	// Search for PROM ID pattern
	_, frame, err := d.FindPromIdPattern(bits)
	if err != nil {
		return nil, err
	}

	return frame, nil
}
