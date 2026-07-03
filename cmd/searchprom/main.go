package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Reading long bit stream and searching for PROM ID [18] [93] at any alignment\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	// Read a long stream of bits (500 bits = 62+ bytes worth)
	const totalBits = 500
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

	fmt.Println("Searching for PROM ID [18] [93] at every possible bit offset...")
	fmt.Println()

	found := false

	// Try every bit offset
	for offset := 0; offset < totalBits-16; offset++ {
		// Extract 2 bytes starting at this offset
		var byte1, byte2 byte

		// First byte (8 bits MSB first)
		for i := 0; i < 8; i++ {
			if offset+i >= totalBits {
				break
			}
			if bits[offset+i] == 1 {
				byte1 |= (1 << (7 - i))
			}
		}

		// Second byte (8 bits MSB first)
		for i := 0; i < 8; i++ {
			if offset+8+i >= totalBits {
				break
			}
			if bits[offset+8+i] == 1 {
				byte2 |= (1 << (7 - i))
			}
		}

		if byte1 == 24 && byte2 == 147 {
			fmt.Printf("✓ FOUND at bit offset %d!\n", offset)
			fmt.Printf("  Bytes: [%02X] [%02X]\n", byte1, byte2)

			// Show context - read more bytes at this alignment
			fmt.Printf("  Frame starting at this offset: ")
			for byteIdx := 0; byteIdx < 20; byteIdx++ {
				var b byte
				bitStart := offset + (byteIdx * 8)
				if bitStart+8 > totalBits {
					break
				}

				for i := 0; i < 8; i++ {
					if bits[bitStart+i] == 1 {
						b |= (1 << (7 - i))
					}
				}

				if byteIdx == 1 || byteIdx == 2 {
					fmt.Printf("[%02X] ", b)
				} else {
					fmt.Printf("%02X ", b)
				}
			}
			fmt.Println()
			fmt.Println()
			found = true
		}
	}

	if !found {
		fmt.Println("❌ PROM ID [18] [93] not found at any offset")
		fmt.Println()
		fmt.Println("First 10 bytes at offset 0:")
		for byteIdx := 0; byteIdx < 10; byteIdx++ {
			var b byte
			for i := 0; i < 8 && (byteIdx*8+i) < totalBits; i++ {
				if bits[byteIdx*8+i] == 1 {
					b |= (1 << (7 - i))
				}
			}
			fmt.Printf("%02X ", b)
		}
		fmt.Println()
	}
}
