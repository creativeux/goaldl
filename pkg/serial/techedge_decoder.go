package serial

import (
	"fmt"
	"goaldl/pkg/errors"
)

// TechEdgeDecoder implements the TechEdge UART decoding approach
// https://www.techedge.com.au/vehicle/aldl160/160serial.htm
//
// Key principle: At 1600 baud, each UART byte period (10 bits @ 625μs each = 6250μs)
// equals one ALDL bit period (160 baud = 6250μs per bit).
//
// Decoding: Read bit 4 (middle data bit) from each UART byte and invert it.
type TechEdgeDecoder struct {
	serial   *AldlSerial
	baudRate int
	Debug    bool
}

// NewTechEdgeDecoder creates a decoder using the TechEdge method
// Recommended baud rates: 1600 (for VN ECUs) or 2400 (for C3 ECUs)
func NewTechEdgeDecoder(portName string, baudRate int) (*TechEdgeDecoder, error) {
	ser, err := NewWithBaudRate(portName, baudRate)
	if err != nil {
		return nil, err
	}

	return &TechEdgeDecoder{
		serial:   ser,
		baudRate: baudRate,
	}, nil
}

// Close closes the serial port
func (d *TechEdgeDecoder) Close() error {
	return d.serial.Close()
}

// ReadAldlBit reads one ALDL bit using the TechEdge method
// At 1600 baud: reads 1 UART byte
// At 4800 baud: reads 3 UART bytes and samples the middle one
func (d *TechEdgeDecoder) ReadAldlBit() (byte, error) {
	var uartByte byte
	var err error

	if d.baudRate == 1600 {
		// At 1600 baud: 1 UART byte = 1 ALDL bit period
		uartByte, err = d.serial.ReadByte()
		if err != nil {
			return 0, err
		}
	} else if d.baudRate == 4800 {
		// At 4800 baud: 3 UART bytes = 1 ALDL bit period
		// Try sampling the first byte instead of middle
		uartByte, err = d.serial.ReadByte() // First byte (sample this one)
		if err != nil {
			return 0, err
		}

		_, err = d.serial.ReadByte() // Second byte
		if err != nil {
			return 0, err
		}

		_, err = d.serial.ReadByte() // Third byte
		if err != nil {
			return 0, err
		}
	} else if d.baudRate == 2400 {
		// At 2400 baud: 1.5 UART bytes = 1 ALDL bit period (approximately)
		// Read 2 bytes and sample the middle one
		uartByte, err = d.serial.ReadByte() // First byte
		if err != nil {
			return 0, err
		}

		_, err = d.serial.ReadByte() // Second byte
		if err != nil {
			return 0, err
		}
	} else {
		// Default: just read one byte
		uartByte, err = d.serial.ReadByte()
		if err != nil {
			return 0, err
		}
	}

	// Extract bit 4 (0-indexed, so bit 3 in code)
	// "Serial data bits 1 through 6 correspond to the mid part of the ALDL data bit.
	//  We could choose to look at serial data bit 4"
	// Bit numbering: bit 0 (LSB) through bit 7 (MSB)
	// We want bit 3 (the 4th bit, 0-indexed)
	bit4 := (uartByte >> 3) & 1

	// Invert due to RS232 polarity inversion
	// "its value is the inverse value of the ALDL data bit"
	aldlBit := 1 - bit4

	if d.Debug {
		fmt.Printf("UART byte: 0x%02X = %08b, bit4=%d -> ALDL bit=%d\n",
			uartByte, uartByte, bit4, aldlBit)
	}

	return aldlBit, nil
}

// ReadAldlByte reads 9 ALDL bits and returns the 8 data bits
// First bit is mode bit (0 for data, 1 for sync)
func (d *TechEdgeDecoder) ReadAldlByte() (byte, byte, error) {
	// Read mode bit
	modeBit, err := d.ReadAldlBit()
	if err != nil {
		return 0, 0, err
	}

	// Read 8 data bits (MSB first)
	var dataByte byte
	for i := 7; i >= 0; i-- {
		bit, err := d.ReadAldlBit()
		if err != nil {
			return 0, 0, err
		}

		if bit == 1 {
			dataByte |= (1 << i)
		}
	}

	if d.Debug {
		fmt.Printf("  ALDL byte: mode=%d data=0x%02X (%d)\n", modeBit, dataByte, dataByte)
	}

	return modeBit, dataByte, nil
}

// FindSyncCharacter searches for 9 consecutive 1-bits (sync character)
func (d *TechEdgeDecoder) FindSyncCharacter() error {
	consecutiveOnes := 0
	maxAttempts := 2000

	if d.Debug {
		fmt.Println("Searching for sync character (9 consecutive 1-bits)...")
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		bit, err := d.ReadAldlBit()
		if err != nil {
			continue
		}

		if bit == 1 {
			consecutiveOnes++
			if d.Debug && consecutiveOnes >= 5 {
				fmt.Printf("  Found %d consecutive 1s...\n", consecutiveOnes)
			}
			if consecutiveOnes >= 9 {
				if d.Debug {
					fmt.Printf("✓ Found sync after %d UART bytes!\n", attempt+1)
				}
				return nil
			}
		} else {
			if d.Debug && consecutiveOnes >= 5 {
				fmt.Printf("  Reset at %d consecutive 1s\n", consecutiveOnes)
			}
			consecutiveOnes = 0
		}
	}

	return errors.NewTimeout("sync character not found")
}

// ReadFrame reads a 20-byte ALDL frame
func (d *TechEdgeDecoder) ReadFrame() ([]byte, error) {
	frame := make([]byte, 20)

	for i := 0; i < 20; i++ {
		_, dataByte, err := d.ReadAldlByte()
		if err != nil {
			return nil, err
		}
		frame[i] = dataByte
	}

	return frame, nil
}

// ReadFrameWithSync finds sync and then reads a frame
func (d *TechEdgeDecoder) ReadFrameWithSync() ([]byte, error) {
	err := d.FindSyncCharacter()
	if err != nil {
		return nil, err
	}

	return d.ReadFrame()
}

// SearchForPromId reads bytes looking for PROM ID pattern
func (d *TechEdgeDecoder) SearchForPromId(maxBytes int) ([]byte, error) {
	buffer := make([]byte, 0, maxBytes)

	if d.Debug {
		fmt.Printf("Searching for PROM ID [24, 147] in up to %d bytes...\n", maxBytes)
	}

	for len(buffer) < maxBytes {
		_, dataByte, err := d.ReadAldlByte()
		if err != nil {
			continue
		}

		buffer = append(buffer, dataByte)

		// Every 10 bytes, show what we've collected
		if d.Debug && len(buffer)%20 == 0 {
			fmt.Printf("Read %d bytes, last 10: ", len(buffer))
			start := len(buffer) - 10
			if start < 0 {
				start = 0
			}
			for j := start; j < len(buffer); j++ {
				fmt.Printf("%d ", buffer[j])
			}
			fmt.Println()
		}

		// Check if we have PROM ID at bytes i+1, i+2
		if len(buffer) >= 3 {
			i := len(buffer) - 3
			if buffer[i+1] == 24 && buffer[i+2] == 147 {
				if d.Debug {
					fmt.Printf("✓ Found PROM ID at byte offset %d\n", i)
					fmt.Printf("  MW2=%d PROMIDA=%d PROMIDB=%d\n",
						buffer[i], buffer[i+1], buffer[i+2])
				}

				// Read the rest of the frame (need 20 bytes total)
				for len(buffer) < i+20 {
					_, dataByte, err := d.ReadAldlByte()
					if err != nil {
						return nil, err
					}
					buffer = append(buffer, dataByte)
				}

				// Return the 20-byte frame
				frame := make([]byte, 20)
				copy(frame, buffer[i:i+20])
				return frame, nil
			}
		}
	}

	// Print final buffer for debugging
	if d.Debug {
		fmt.Printf("Buffer final length: %d\n", len(buffer))
		fmt.Print("Last 30 bytes: ")
		start := len(buffer) - 30
		if start < 0 {
			start = 0
		}
		for j := start; j < len(buffer); j++ {
			fmt.Printf("%d ", buffer[j])
		}
		fmt.Println()
	}

	return nil, errors.NewInvalidFrame("PROM ID not found")
}
