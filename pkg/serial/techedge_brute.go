package serial

import (
	"fmt"
)

// BruteForceDecode tries different sampling strategies at 4800 baud
// to find which one produces the PROM ID [24, 147]
type BruteForceDecode struct {
	serial *AldlSerial
}

// NewBruteForceDecode creates a brute force decoder
func NewBruteForceDecode(portName string) (*BruteForceDecode, error) {
	ser, err := NewWithBaudRate(portName, 4800)
	if err != nil {
		return nil, err
	}

	return &BruteForceDecode{
		serial: ser,
	}, nil
}

// Close closes the serial port
func (b *BruteForceDecode) Close() error {
	return b.serial.Close()
}

// TestConfig tests a specific decoding configuration
type TestConfig struct {
	ByteToSample int  // 0=first, 1=middle, 2=last of 3 bytes
	BitToExtract int  // 0-7, which bit to extract
	Invert       bool // whether to invert the bit
	Description  string
}

// TryConfiguration tests one specific configuration
func (b *BruteForceDecode) TryConfiguration(config TestConfig, numBytes int) ([]byte, error) {
	buffer := make([]byte, 0, numBytes)

	for len(buffer) < numBytes {
		// Read 3 UART bytes (one ALDL bit period at 4800 baud)
		bytes := make([]byte, 3)
		for i := 0; i < 3; i++ {
			b, err := b.serial.ReadByte()
			if err != nil {
				return nil, err
			}
			bytes[i] = b
		}

		// Sample the configured byte
		sampleByte := bytes[config.ByteToSample]

		// Extract the configured bit
		bit := (sampleByte >> config.BitToExtract) & 1

		// Invert if configured
		if config.Invert {
			bit = 1 - bit
		}

		// Accumulate 9 bits into one ALDL byte
		// This is simplified - just accumulating into bytes for now
		// In reality we'd need to track the 9-bit boundary

		// For testing, let's just accumulate 8 bits
		if len(buffer) == 0 || len(buffer)%8 == 0 {
			buffer = append(buffer, 0)
		}

		// Shift bit into current byte
		byteIndex := len(buffer) - 1
		bitPos := (len(buffer) * 8) % 8
		if bit == 1 {
			buffer[byteIndex] |= (1 << (7 - bitPos))
		}
	}

	return buffer, nil
}

// FindBestConfiguration tries all configurations and finds which produces PROM ID
func (b *BruteForceDecode) FindBestConfiguration(verbose bool) *TestConfig {
	configs := []TestConfig{
		{0, 3, true, "First byte, bit 3 (4), inverted"},
		{0, 4, true, "First byte, bit 4 (5), inverted"},
		{0, 5, true, "First byte, bit 5 (6), inverted"},
		{1, 3, true, "Middle byte, bit 3 (4), inverted"},
		{1, 4, true, "Middle byte, bit 4 (5), inverted"},
		{1, 5, true, "Middle byte, bit 5 (6), inverted"},
		{2, 3, true, "Last byte, bit 3 (4), inverted"},
		{2, 4, true, "Last byte, bit 4 (5), inverted"},
		{2, 5, true, "Last byte, bit 5 (6), inverted"},
		{0, 3, false, "First byte, bit 3 (4), not inverted"},
		{1, 3, false, "Middle byte, bit 3 (4), not inverted"},
		{2, 3, false, "Last byte, bit 3 (4), not inverted"},
	}

	for _, config := range configs {
		if verbose {
			fmt.Printf("Testing: %s\n", config.Description)
		}

		// Read 100 bytes with this config
		data, err := b.TryConfiguration(config, 100)
		if err != nil {
			continue
		}

		// Look for [24, 147] pattern
		for i := 0; i < len(data)-20; i++ {
			if data[i+1] == 24 && data[i+2] == 147 {
				fmt.Printf("✓ FOUND! %s\n", config.Description)
				fmt.Printf("  Frame starting at byte %d: ", i)
				for j := i; j < i+20 && j < len(data); j++ {
					fmt.Printf("%d ", data[j])
				}
				fmt.Println()
				return &config
			}
		}

		if verbose {
			fmt.Print("  First 20 bytes: ")
			for i := 0; i < 20 && i < len(data); i++ {
				fmt.Printf("%d ", data[i])
			}
			fmt.Println()
		}
	}

	return nil
}
