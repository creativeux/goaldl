package serial

import (
	"fmt"
	"goaldl/pkg/errors"
)

// BitCountDecoder uses bit density counting instead of sampling specific bits
// Based on Opus's suggestion - more robust for varying hardware timing
type BitCountDecoder struct {
	serial   *AldlSerial
	baudRate int
	Debug    bool
}

// NewBitCountDecoder creates a decoder that counts '1' bits
func NewBitCountDecoder(portName string, baudRate int) (*BitCountDecoder, error) {
	ser, err := NewWithBaudRate(portName, baudRate)
	if err != nil {
		return nil, err
	}

	return &BitCountDecoder{
		serial:   ser,
		baudRate: baudRate,
	}, nil
}

// Close closes the serial port
func (d *BitCountDecoder) Close() error {
	return d.serial.Close()
}

// countOnes counts the number of '1' bits in a byte
func countOnes(b byte) int {
	count := 0
	for i := 0; i < 8; i++ {
		if (b & (1 << i)) != 0 {
			count++
		}
	}
	return count
}

// ReadAldlBit reads one ALDL bit by counting '1' bits in UART bytes
func (d *BitCountDecoder) ReadAldlBit() (byte, error) {
	var totalOnes int
	var bytesRead int

	if d.baudRate == 4800 {
		// At 4800 baud: read 3 UART bytes per ALDL bit
		// Count total '1' bits across all 3 bytes
		for i := 0; i < 3; i++ {
			b, err := d.serial.ReadByte()
			if err != nil {
				return 0, err
			}
			totalOnes += countOnes(b)
			bytesRead++
		}

		// Threshold: out of 24 bits (3 bytes), how many are '1'?
		// Based on 1227747 timing (from Bot Thoughts and Opus):
		// ALDL 0 bit (~368μs low pulse): line HIGH most of time = more '1's in UART (0xFE, 0xFF)
		// ALDL 1 bit (~4400μs low pulse): line LOW most of time = more '0's in UART (0x07, 0x0F)
		//
		// Therefore: more '1's = ALDL 0, fewer '1's = ALDL 1

		// Threshold at 12 (half of 24 bits)
		if totalOnes >= 12 {
			if d.Debug {
				fmt.Printf("UART bytes: %d ones / 24 bits -> ALDL bit 0 (short pulse)\n", totalOnes)
			}
			return 0, nil
		} else {
			if d.Debug {
				fmt.Printf("UART bytes: %d ones / 24 bits -> ALDL bit 1 (long pulse)\n", totalOnes)
			}
			return 1, nil
		}

	} else if d.baudRate == 1600 {
		// At 1600 baud: 1 UART byte per ALDL bit
		b, err := d.serial.ReadByte()
		if err != nil {
			return 0, err
		}
		totalOnes = countOnes(b)

		// Threshold at 4 (half of 8 bits)
		if totalOnes >= 5 {
			return 1, nil
		} else {
			return 0, nil
		}

	} else {
		// Default: just read one byte
		b, err := d.serial.ReadByte()
		if err != nil {
			return 0, err
		}
		totalOnes = countOnes(b)

		if totalOnes >= 4 {
			return 1, nil
		} else {
			return 0, nil
		}
	}
}

// ReadAldlByte reads 9 ALDL bits (1 mode + 8 data), returns mode bit and data byte
func (d *BitCountDecoder) ReadAldlByte() (byte, byte, error) {
	// Read mode bit (first bit)
	modeBit, err := d.ReadAldlBit()
	if err != nil {
		return 0, 0, err
	}

	// Read 8 data bits (MSB first per ALDL spec)
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

// FindSyncCharacter searches for 9 consecutive '1' bits
func (d *BitCountDecoder) FindSyncCharacter() error {
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
					fmt.Printf("✓ Found sync after %d attempts!\n", attempt+1)
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
func (d *BitCountDecoder) ReadFrame() ([]byte, error) {
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

// ReadFrameWithSync finds sync then reads frame
func (d *BitCountDecoder) ReadFrameWithSync() ([]byte, error) {
	err := d.FindSyncCharacter()
	if err != nil {
		return nil, err
	}

	return d.ReadFrame()
}

// SearchForPromId reads bytes looking for PROM ID [24, 147]
func (d *BitCountDecoder) SearchForPromId(maxBytes int) ([]byte, error) {
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

		// Show progress
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

		// Check for PROM ID at bytes i+1, i+2
		if len(buffer) >= 3 {
			i := len(buffer) - 3
			if buffer[i+1] == 24 && buffer[i+2] == 147 {
				if d.Debug {
					fmt.Printf("✓ Found PROM ID at byte offset %d\n", i)
					fmt.Printf("  MW2=%d PROMIDA=%d PROMIDB=%d\n",
						buffer[i], buffer[i+1], buffer[i+2])
				}

				// Read the rest of the frame
				for len(buffer) < i+20 {
					_, dataByte, err := d.ReadAldlByte()
					if err != nil {
						return nil, err
					}
					buffer = append(buffer, dataByte)
				}

				frame := make([]byte, 20)
				copy(frame, buffer[i:i+20])
				return frame, nil
			}
		}
	}

	return nil, errors.NewInvalidFrame("PROM ID not found")
}
