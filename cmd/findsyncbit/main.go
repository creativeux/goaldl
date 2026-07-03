package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Searching for ALDL sync character (9 consecutive 1 bits)\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	// Read bits and look for 9 consecutive 1s
	const windowSize = 500
	bits := make([]byte, windowSize)

	fmt.Printf("Reading %d bits and searching for sync pattern (9 consecutive 1s)...\n\n", windowSize)

	for i := 0; i < windowSize; i++ {
		bit, err := ser.ReadAldlBitRunLength()
		if err != nil {
			fmt.Printf("Error at bit %d: %v\n", i, err)
			return
		}
		bits[i] = bit

		// Check if we have 9 consecutive 1s ending at this position
		if i >= 8 {
			allOnes := true
			for j := 0; j < 9; j++ {
				if bits[i-8+j] != 1 {
					allOnes = false
					break
				}
			}

			if allOnes {
				fmt.Printf("✓ SYNC FOUND at bit position %d!\n", i-8)
				fmt.Printf("  Pattern: ")
				for j := i - 8; j <= i; j++ {
					fmt.Printf("%d", bits[j])
				}
				fmt.Printf(" (9 consecutive 1s)\n")

				// Read the next 160 bits (20 bytes) after sync
				fmt.Printf("\n  Reading frame after sync (160 bits = 20 bytes):\n")
				frame := make([]byte, 20)

				for byteIdx := 0; byteIdx < 20; byteIdx++ {
					var b byte
					for bitIdx := 7; bitIdx >= 0; bitIdx-- {
						bit, err := ser.ReadAldlBitRunLength()
						if err != nil {
							fmt.Printf("Error reading frame: %v\n", err)
							return
						}

						if bit == 1 {
							b |= (1 << bitIdx)
						}
					}
					frame[byteIdx] = b
				}

				fmt.Printf("  Frame: ")
				for idx, b := range frame {
					if idx == 1 || idx == 2 {
						fmt.Printf("[%02X] ", b)
					} else {
						fmt.Printf("%02X ", b)
					}
				}

				if frame[1] == 24 && frame[2] == 147 {
					fmt.Printf("\n  ✓✓✓ PROM ID MATCH! [%02X] [%02X]\n", frame[1], frame[2])
				} else {
					fmt.Printf("\n  PROM ID mismatch: expected [18] [93], got [%02X] [%02X]\n", frame[1], frame[2])
				}
				fmt.Println()

				// Reset and continue searching
				i = 0
				for j := 0; j < windowSize; j++ {
					bits[j] = 0
				}
			}
		}
	}

	fmt.Println("\nSearch complete")
}
