package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Searching for PROM ID pattern in bit stream\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	// Read a large number of bits (300 bits = ~37 bytes worth, more than a frame)
	const totalBits = 400
	bits := make([]byte, totalBits)

	fmt.Printf("Reading %d bits...\n", totalBits)
	for i := 0; i < totalBits; i++ {
		bit, err := ser.ReadAldlBitRunLength()
		if err != nil {
			fmt.Printf("Error at bit %d: %v\n", i, err)
			return
		}
		bits[i] = bit
	}

	fmt.Println("Done reading bits")
	fmt.Println()

	// Try every possible 8-bit alignment to find PROM ID [24, 147]
	fmt.Println("Searching for PROM ID pattern [24, 147] = [0x18, 0x93] at different alignments:")
	fmt.Println()

	for offset := 0; offset < totalBits-160; offset++ { // 160 bits = 20 bytes
		// Extract 20 bytes starting from this offset
		frame := make([]byte, 20)
		for byteIdx := 0; byteIdx < 20; byteIdx++ {
			var b byte
			bitOffset := offset + (byteIdx * 8)

			// Read 8 bits MSB first
			for bitIdx := 0; bitIdx < 8; bitIdx++ {
				if bitOffset+bitIdx >= totalBits {
					break
				}
				if bits[bitOffset+bitIdx] == 1 {
					b |= (1 << (7 - bitIdx))
				}
			}
			frame[byteIdx] = b
		}

		// Check if bytes 1-2 match PROM ID (byte 0 is MW2 which varies)
		if frame[1] == 24 && frame[2] == 147 {
			fmt.Printf("✓ FOUND at bit offset %d!\n", offset)
			fmt.Printf("  Frame (20 bytes): ")
			for i, b := range frame {
				if i == 1 || i == 2 {
					fmt.Printf("[%02X] ", b)
				} else {
					fmt.Printf("%02X ", b)
				}
			}
			fmt.Println()

			// Show the bit pattern for this frame
			fmt.Printf("  Bit pattern: ")
			for i := 0; i < 160 && offset+i < totalBits; i++ {
				if i > 0 && i%8 == 0 {
					fmt.Print(" ")
				}
				fmt.Printf("%d", bits[offset+i])
			}
			fmt.Println()
			fmt.Println()
		}
	}

	fmt.Println("Search complete")
}
