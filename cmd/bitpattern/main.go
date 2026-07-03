package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Showing bit patterns and looking for consecutive 1s\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	const totalBits = 300
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

	// Display bits in groups of 8 (byte-aligned)
	fmt.Println("\nBit stream (grouped by 8):")
	for i := 0; i < totalBits; i++ {
		if i > 0 && i%8 == 0 {
			fmt.Print(" ")
		}
		if i > 0 && i%80 == 0 {
			fmt.Println()
		}
		fmt.Printf("%d", bits[i])
	}
	fmt.Println()

	// Count consecutive 1s
	fmt.Println("\nLongest runs of consecutive 1s:")
	maxRun := 0
	currentRun := 0
	runs := make(map[int]int) // run length -> count

	for i := 0; i < totalBits; i++ {
		if bits[i] == 1 {
			currentRun++
			if currentRun > maxRun {
				maxRun = currentRun
			}
		} else {
			if currentRun > 0 {
				runs[currentRun]++
				if currentRun >= 5 {
					// Show position of long runs
					fmt.Printf("  %d consecutive 1s ending at bit %d\n", currentRun, i-1)
				}
				currentRun = 0
			}
		}
	}

	fmt.Printf("\nMaximum consecutive 1s seen: %d\n", maxRun)
	fmt.Println("\nDistribution of consecutive 1s:")
	for length := 1; length <= maxRun; length++ {
		if runs[length] > 0 {
			fmt.Printf("  %d consecutive 1s: %d occurrences\n", length, runs[length])
		}
	}

	if maxRun < 9 {
		fmt.Println("\n⚠️  Never saw 9 consecutive 1s (sync character)")
		fmt.Println("This could mean:")
		fmt.Println("  1. Threshold is wrong (miscategorizing some 1s as 0s)")
		fmt.Println("  2. We're not sampling at the right baud rate")
		fmt.Println("  3. ECM is not transmitting data continuously")
	}
}
