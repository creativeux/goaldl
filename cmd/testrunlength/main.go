package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Testing run-length decoder on %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	// Read 10 ALDL bytes (8 bits each, MSB first)
	fmt.Println("Reading ALDL bytes (8 bits each, MSB first):")
	fmt.Println()

	for byteNum := 0; byteNum < 10; byteNum++ {
		var aldlByte byte

		fmt.Printf("Byte %d: ", byteNum)

		// Read 8 bits, MSB first
		for bitPos := 7; bitPos >= 0; bitPos-- {
			bit, err := ser.ReadAldlBitRunLength()
			if err != nil {
				fmt.Printf("\n  ERROR reading bit %d: %v\n", 7-bitPos, err)
				break
			}

			// Set bit in position
			if bit == 1 {
				aldlByte |= (1 << bitPos)
			}

			fmt.Printf("%d", bit)
		}

		fmt.Printf(" = 0x%02X (%3d)\n", aldlByte, aldlByte)
	}

	fmt.Println()
	fmt.Println("Expected PROM ID at bytes 1-2: [24, 147] = [0x18, 0x93]")
}
