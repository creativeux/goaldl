package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Analyzing pulse sequences for sync pattern\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	fmt.Println("Reading pulse widths and looking for patterns...")
	fmt.Println("Legend: S=short (2-4 bytes), L=long (5+ bytes), G=glitch (1 byte)")
	fmt.Println()

	// Read pulse widths and classify them
	const totalPulses = 300
	pulses := make([]int, totalPulses)

	for i := 0; i < totalPulses; i++ {
		runLength, err := ser.MeasurePulseWidthRunLength()
		if err != nil {
			continue
		}
		pulses[i] = runLength
	}

	// Display pulse sequence
	fmt.Println("Pulse sequence:")
	for i := 0; i < totalPulses; i++ {
		if i > 0 && i%80 == 0 {
			fmt.Println()
		}

		p := pulses[i]
		if p == 1 {
			fmt.Print("G")
		} else if p >= 2 && p <= 4 {
			fmt.Print("S")
		} else {
			fmt.Print("L")
		}
	}
	fmt.Println()
	fmt.Println()

	// Look for sequences of L's (ignoring G's)
	fmt.Println("Looking for long sequences of L (long pulses)...")
	maxLongRun := 0
	currentLongRun := 0

	for i := 0; i < totalPulses; i++ {
		p := pulses[i]

		if p >= 5 {
			// Long pulse
			currentLongRun++
			if currentLongRun > maxLongRun {
				maxLongRun = currentLongRun
			}
		} else if p >= 2 && p <= 4 {
			// Short pulse - breaks the run
			if currentLongRun >= 5 {
				fmt.Printf("  Found %d consecutive long pulses ending at position %d\n", currentLongRun, i-1)
			}
			currentLongRun = 0
		}
		// Glitches (p==1) are ignored, don't break the run
	}

	fmt.Printf("\nMaximum consecutive long pulses: %d\n", maxLongRun)

	if maxLongRun >= 9 {
		fmt.Println("✓ Found potential sync pattern (9+ consecutive long pulses)!")
	} else {
		fmt.Printf("⚠️  Only found %d consecutive long pulses, need 9 for sync\n", maxLongRun)
		fmt.Println()
		fmt.Println("Possible issues:")
		fmt.Println("  1. Short pulses are interrupting what should be long runs")
		fmt.Println("  2. Threshold between short/long is wrong")
		fmt.Println("  3. Baud rate is incorrect")
		fmt.Println("  4. Sync is encoded differently than expected")
	}
}
