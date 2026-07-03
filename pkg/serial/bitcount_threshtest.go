package serial

import (
	"fmt"
)

// ThresholdTester tests different bit-count thresholds
type ThresholdTester struct {
	serial *AldlSerial
}

// NewThresholdTester creates a threshold tester
func NewThresholdTester(portName string, baudRate int) (*ThresholdTester, error) {
	ser, err := NewWithBaudRate(portName, baudRate)
	if err != nil {
		return nil, err
	}

	return &ThresholdTester{
		serial: ser,
	}, nil
}

// Close closes the serial port
func (t *ThresholdTester) Close() error {
	return t.serial.Close()
}

// TestThresholds tries different threshold values
func (t *ThresholdTester) TestThresholds(numBytes int) {
	fmt.Println("Testing different bit-count thresholds at 4800 baud")
	fmt.Println("Each test collects", numBytes, "bytes and checks for value 24")
	fmt.Println()

	// Collect raw UART bytes first
	rawBytes := make([]byte, numBytes*3*9) // 3 UART bytes per ALDL bit, 9 bits per ALDL byte
	for i := 0; i < len(rawBytes); i++ {
		b, err := t.serial.ReadByte()
		if err != nil {
			fmt.Printf("Error reading byte %d: %v\n", i, err)
			return
		}
		rawBytes[i] = b
	}

	fmt.Printf("Collected %d raw UART bytes\n\n", len(rawBytes))

	// Try thresholds from 6 to 18
	for threshold := 6; threshold <= 18; threshold++ {
		fmt.Printf("Testing threshold %d (out of 24 bits):\n", threshold)

		// Decode using this threshold
		buffer := make([]byte, 0, numBytes)
		rawIdx := 0

		for len(buffer) < numBytes && rawIdx+27 < len(rawBytes) {
			// Read one ALDL byte = 9 ALDL bits
			var aldlByte byte
			for bit := 0; bit < 9; bit++ {
				// Count ones in next 3 UART bytes
				if rawIdx+3 > len(rawBytes) {
					break
				}

				totalOnes := countOnes(rawBytes[rawIdx]) +
					countOnes(rawBytes[rawIdx+1]) +
					countOnes(rawBytes[rawIdx+2])
				rawIdx += 3

				// Apply threshold - CORRECTED LOGIC
				// More 1s = ALDL 0 (short pulse), More 0s = ALDL 1 (long pulse)
				aldlBit := byte(0)
				if totalOnes >= threshold {
					aldlBit = 0 // ALDL bit 0
				} else {
					aldlBit = 1 // ALDL bit 1
				}

				// Skip mode bit (first bit)
				if bit > 0 {
					aldlByte = (aldlByte << 1) | aldlBit
				}
			}

			buffer = append(buffer, aldlByte)
		}

		// Check if we found value 24
		count24 := 0
		for _, b := range buffer {
			if b == 24 {
				count24++
			}
		}

		// Check for [24, 147] pattern
		foundPattern := false
		for i := 0; i < len(buffer)-1; i++ {
			if buffer[i] == 24 && buffer[i+1] == 147 {
				foundPattern = true
				break
			}
		}

		fmt.Printf("  First 20 bytes: ")
		for i := 0; i < 20 && i < len(buffer); i++ {
			fmt.Printf("%d ", buffer[i])
		}
		fmt.Printf("\n  Found %d occurrences of value 24", count24)
		if foundPattern {
			fmt.Print(" ✓ FOUND [24, 147]!")
		}
		fmt.Println()
		fmt.Println()
	}
}
