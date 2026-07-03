package main

import (
	"fmt"
	"log"
	"sort"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Pulse width histogram from %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	// Collect pulse widths
	const sampleCount = 200
	pulseWidths := make(map[int]int)

	fmt.Printf("Sampling %d pulses...\n", sampleCount)
	for i := 0; i < sampleCount; i++ {
		runLength, err := ser.MeasurePulseWidthRunLength()
		if err != nil {
			continue
		}
		pulseWidths[runLength]++
	}

	// Sort by run length
	var lengths []int
	for length := range pulseWidths {
		lengths = append(lengths, length)
	}
	sort.Ints(lengths)

	fmt.Println()
	fmt.Println("Pulse Width Distribution:")
	fmt.Println("Length (bytes) | Duration (μs) | Count | Bar | Classification")
	fmt.Println("---------------|---------------|-------|-----|---------------")

	for _, length := range lengths {
		count := pulseWidths[length]
		duration := length * 208
		bar := ""
		for j := 0; j < count && j < 50; j++ {
			bar += "█"
		}

		classification := ""
		if length == 1 {
			classification = "NOISE (skip)"
		} else if length >= 2 && length <= 5 {
			classification = "Bit 0 (short)"
		} else if length >= 6 && length <= 7 {
			classification = "Ambiguous"
		} else {
			classification = "Bit 1 (long)"
		}

		fmt.Printf("%14d | %13d | %5d | %s %s\n",
			length, duration, count, bar, classification)
	}

	fmt.Println()
	fmt.Println("Expected for ALDL at 4800 baud:")
	fmt.Println("  Bit 0: 360-370μs ≈ 2 bytes")
	fmt.Println("  Bit 1: 1850-4400μs ≈ 9-21 bytes")
	fmt.Println()
	fmt.Println("Current threshold: 2-5 bytes = bit 0, 6+ bytes = bit 1")
}
