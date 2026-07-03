package serial

import (
	"fmt"
	"goaldl/pkg/errors"
)

// PulseWidthDecoder decodes ALDL by measuring actual pulse widths across UART bytes
type PulseWidthDecoder struct {
	serial      *AldlSerial
	baudRate    int
	bitBuffer   []byte // Buffer of bits read from UART
	bitPosition int    // Current position in bit buffer
	Debug       bool   // Enable debug output
}

// NewPulseWidthDecoder creates a decoder that measures pulse widths
func NewPulseWidthDecoder(portName string, baudRate int) (*PulseWidthDecoder, error) {
	ser, err := NewWithBaudRate(portName, baudRate)
	if err != nil {
		return nil, err
	}

	return &PulseWidthDecoder{
		serial:      ser,
		baudRate:    baudRate,
		bitBuffer:   make([]byte, 0, 1024),
		bitPosition: 0,
	}, nil
}

// Close closes the serial port
func (d *PulseWidthDecoder) Close() error {
	return d.serial.Close()
}

// ReadAldlBit reads one ALDL bit using bot-thoughts sampling approach
// Instead of measuring full pulse width, sample at fixed time after falling edge
func (d *PulseWidthDecoder) ReadAldlBit() (byte, error) {
	// At 4800 baud: 1 UART bit = 208.33 μs
	// Bot-thoughts samples at 2000 μs after falling edge
	// 2000 μs ÷ 208.33 μs = 9.6 UART bits
	//
	// Logic 0: 360 μs pulse → signal returns HIGH before sample
	// Logic 1: 1850 μs pulse → signal still LOW at sample
	// Sampling at 1000 μs (between them) might give better discrimination
	// 1000 μs ÷ 208.33 μs = 4.8 bits → use 5
	const sampleOffset = 5 // Sample 5 UART bits after falling edge (~1040 μs)

	// Ensure we have enough bits buffered
	// Need at least sampleOffset bits after current position, plus buffer for next edge
	for len(d.bitBuffer)-d.bitPosition < 40 {
		if err := d.bufferMoreBits(); err != nil {
			return 0, err
		}
	}

	// Find next falling edge (start of pulse)
	edgePos := d.findFallingEdge()
	if edgePos < 0 {
		// No edge found, need more data
		d.bitPosition = len(d.bitBuffer) - 10 // Keep last few bits
		return 0, errors.NewInvalidFrame("no falling edge found")
	}

	// Move position to sample point (2000 μs after edge)
	samplePos := edgePos + sampleOffset

	// Ensure we have data at sample position
	for samplePos >= len(d.bitBuffer) {
		if err := d.bufferMoreBits(); err != nil {
			return 0, err
		}
	}

	// Sample the signal level at fixed time after edge
	// LOW at sample point = logic 1 (long pulse still active)
	// HIGH at sample point = logic 0 (short pulse already ended)
	var bit byte
	if d.bitBuffer[samplePos] == 0 {
		bit = 1 // Signal still LOW = logic 1
	} else {
		bit = 0 // Signal back HIGH = logic 0
	}

	if d.Debug {
		// Show context around the edge and sample point
		start := edgePos - 2
		if start < 0 {
			start = 0
		}
		end := samplePos + 3
		if end > len(d.bitBuffer) {
			end = len(d.bitBuffer)
		}
		fmt.Printf("Edge@%d Sample@%d [", edgePos, samplePos)
		for i := start; i < end; i++ {
			if i == edgePos {
				fmt.Print("|")
			}
			if i == samplePos {
				fmt.Print("*")
			}
			fmt.Printf("%d", d.bitBuffer[i])
		}
		fmt.Printf("] -> %d\n", bit)
	}

	// Advance position past this pulse
	// Skip to the sample point, then look for next edge
	d.bitPosition = samplePos

	return bit, nil
}

// ReadAldlByte reads 9 bits (1 mode bit + 8 data bits), returns the 8 data bits
func (d *PulseWidthDecoder) ReadAldlByte() (byte, error) {
	// Read mode bit (should be 0 for data, 1 for sync)
	modeBit, err := d.ReadAldlBit()
	if err != nil {
		return 0, err
	}

	// If mode bit is 1, this might be part of sync character
	_ = modeBit // We'll handle sync separately

	// Read 8 data bits MSB first
	var result byte
	for i := 7; i >= 0; i-- {
		bit, err := d.ReadAldlBit()
		if err != nil {
			return 0, err
		}

		if bit == 1 {
			result |= (1 << i)
		}
	}

	if d.Debug {
		fmt.Printf("  ReadAldlByte: mode=%d data=0x%02X (%d)\n", modeBit, result, result)
	}

	return result, nil
}

// FindSyncCharacter searches for 9 consecutive logic-1 bits (0x1FF sync pattern)
func (d *PulseWidthDecoder) FindSyncCharacter() error {
	consecutiveOnes := 0
	maxAttempts := 1000
	bitHistory := make([]byte, 0, 20)

	if d.Debug {
		fmt.Println("Searching for sync character (9 consecutive 1s)...")
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		bit, err := d.ReadAldlBit()
		if err != nil {
			continue
		}

		bitHistory = append(bitHistory, bit)
		if len(bitHistory) > 20 {
			bitHistory = bitHistory[1:]
		}

		if bit == 1 {
			consecutiveOnes++
			if d.Debug && consecutiveOnes >= 3 {
				fmt.Printf("  Found %d consecutive 1s...\n", consecutiveOnes)
			}
			if consecutiveOnes >= 9 {
				// Found sync! We're now at a frame boundary
				if d.Debug {
					fmt.Printf("✓ Found sync character after %d attempts!\n", attempt+1)
					fmt.Print("  Last 20 bits: ")
					for _, b := range bitHistory {
						fmt.Printf("%d", b)
					}
					fmt.Println()
				}
				return nil
			}
		} else {
			if d.Debug && consecutiveOnes >= 3 {
				fmt.Printf("  Reset at %d consecutive 1s (bit 0 encountered)\n", consecutiveOnes)
			}
			consecutiveOnes = 0
		}
	}

	if d.Debug {
		fmt.Printf("✗ Sync not found after %d attempts\n", maxAttempts)
		fmt.Print("  Last 20 bits: ")
		for _, b := range bitHistory {
			fmt.Printf("%d", b)
		}
		fmt.Println()
	}

	return errors.NewTimeout("sync character not found")
}

// ReadFrame reads a 20-byte ALDL frame
func (d *PulseWidthDecoder) ReadFrame() ([]byte, error) {
	frame := make([]byte, 20)

	for i := 0; i < 20; i++ {
		b, err := d.ReadAldlByte()
		if err != nil {
			return nil, err
		}
		frame[i] = b
	}

	return frame, nil
}

// bufferMoreBits reads UART bytes and adds them to the bit buffer
func (d *PulseWidthDecoder) bufferMoreBits() error {
	// Read a chunk of UART bytes
	uartByte, err := d.serial.ReadByte()
	if err != nil {
		return err
	}

	// Convert byte to 8 bits and append to buffer
	// MSB first
	for i := 7; i >= 0; i-- {
		if (uartByte & (1 << i)) != 0 {
			d.bitBuffer = append(d.bitBuffer, 1)
		} else {
			d.bitBuffer = append(d.bitBuffer, 0)
		}
	}

	return nil
}

// findFallingEdge finds the next high→low transition
// Returns position, or -1 if not found
func (d *PulseWidthDecoder) findFallingEdge() int {
	// ALDL idle state is HIGH
	// Pulse starts with falling edge (high→low)

	for i := d.bitPosition; i < len(d.bitBuffer)-1; i++ {
		if d.bitBuffer[i] == 1 && d.bitBuffer[i+1] == 0 {
			return i + 1 // Return position of low bit (start of pulse)
		}
	}

	return -1
}

// measurePulseWidth measures how many bits the signal stays low
func (d *PulseWidthDecoder) measurePulseWidth() int {
	count := 0
	pos := d.bitPosition

	// Count consecutive 0 bits
	for pos < len(d.bitBuffer) && d.bitBuffer[pos] == 0 {
		count++
		pos++
	}

	// Advance position past the pulse
	d.bitPosition = pos

	return count
}

// SyncToByte attempts to find byte alignment by looking for sync pattern
// The ALDL sync character is 9 consecutive 1 bits (0x1FF)
func (d *PulseWidthDecoder) SyncToByte() error {
	// Look for a pattern that indicates byte boundary
	// This is challenging without a true sync character
	// For now, just try to read and hope for the best

	// Read some bits to get into the stream
	for i := 0; i < 100; i++ {
		_, err := d.ReadAldlBit()
		if err != nil {
			continue
		}
	}

	return nil
}

// ReadFrameWithSync attempts to find sync character and then read a frame
func (d *PulseWidthDecoder) ReadFrameWithSync() ([]byte, error) {
	// Find the sync character (9 consecutive ones)
	err := d.FindSyncCharacter()
	if err != nil {
		return nil, err
	}

	// Now we're aligned to frame boundary, read 20 bytes
	frame, err := d.ReadFrame()
	if err != nil {
		return nil, err
	}

	return frame, nil
}

// ReadByteStreamAndSearch reads a large buffer and searches for PROM ID
func (d *PulseWidthDecoder) ReadByteStreamAndSearch(numBytes int) ([]byte, int, error) {
	buffer := make([]byte, 0, numBytes)

	// Read bytes into buffer
	for i := 0; i < numBytes; i++ {
		b, err := d.ReadAldlByte()
		if err != nil {
			if len(buffer) < 20 {
				return nil, -1, err
			}
			break // Use what we have
		}
		buffer = append(buffer, b)
	}

	// Search for PROM ID [24, 147] at all possible offsets
	for offset := 0; offset < len(buffer)-20; offset++ {
		// Check if bytes at offset+1 and offset+2 match PROM ID
		if buffer[offset+1] == 24 && buffer[offset+2] == 147 {
			// Found it! Extract 20-byte frame starting at offset
			frame := buffer[offset : offset+20]
			return frame, offset, nil
		}
	}

	return nil, -1, errors.NewInvalidFrame("PROM ID not found in buffer")
}
