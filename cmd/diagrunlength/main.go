package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Diagnostic run-length decoder on %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	fmt.Println("Reading 80 ALDL bits and showing run-lengths:")
	fmt.Println()

	for i := 0; i < 80; i++ {
		runLength, err := ser.MeasurePulseWidthRunLength()
		if err != nil {
			fmt.Printf("Bit %3d: ERROR: %v\n", i, err)
			continue
		}

		bit, _ := ser.DecodeAldlBitRunLength(runLength)
		duration := runLength * 208 // microseconds

		classification := "?"
		if runLength <= 6 {
			classification = "SHORT (bit 0)"
		} else {
			classification = "LONG (bit 1)"
		}

		fmt.Printf("Bit %3d: %2d bytes ≈ %4dμs -> %s -> decoded as %d\n",
			i, runLength, duration, classification, bit)

		// Print byte boundaries
		if (i+1) % 8 == 0 {
			fmt.Println()
		}
	}

	fmt.Println("\nExpected patterns:")
	fmt.Println("  PROM ID byte 1 = 24 (0x18) = 00011000")
	fmt.Println("  PROM ID byte 2 = 147 (0x93) = 10010011")
}
