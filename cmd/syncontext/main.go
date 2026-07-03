package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Examining pulse sequence AROUND the 40-byte pulses (potential sync)\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	fmt.Println("Waiting for a 40-byte pulse, then showing context...")
	fmt.Println()

	syncCount := 0

	for syncCount < 3 {
		// Read pulses until we find a ~40-byte one
		pulseBuffer := make([]int, 0, 30)

		for {
			runLength, err := ser.MeasurePulseWidthRunLength()
			if err != nil {
				continue
			}

			// Skip 1-byte glitches from buffer
			if runLength > 1 {
				pulseBuffer = append(pulseBuffer, runLength)

				// Keep buffer at last 20 pulses
				if len(pulseBuffer) > 20 {
					pulseBuffer = pulseBuffer[1:]
				}
			}

			// Check if this is a long pulse (potential sync)
			if runLength >= 30 {
				fmt.Printf("=== SYNC CANDIDATE #%d: %d-byte pulse (%dμs) ===\n",
					syncCount+1, runLength, runLength*208)

				// Show 10 pulses before
				fmt.Println("Pulses BEFORE:")
				start := len(pulseBuffer) - 11
				if start < 0 {
					start = 0
				}
				for i := start; i < len(pulseBuffer)-1; i++ {
					p := pulseBuffer[i]
					classification := "SHORT"
					if p >= 5 {
						classification = "LONG"
					}
					fmt.Printf("  %2d bytes (%4dμs) - %s\n", p, p*208, classification)
				}

				fmt.Printf(">>> %2d bytes (%4dμs) - SYNC CANDIDATE <<<\n", runLength, runLength*208)

				// Read next 10 pulses after
				fmt.Println("Pulses AFTER:")
				for i := 0; i < 10; i++ {
					nextPulse, err := ser.MeasurePulseWidthRunLength()
					if err != nil {
						continue
					}
					if nextPulse == 1 {
						i-- // don't count glitches
						continue
					}

					classification := "SHORT"
					if nextPulse >= 5 {
						classification = "LONG"
					}
					fmt.Printf("  %2d bytes (%4dμs) - %s\n", nextPulse, nextPulse*208, classification)
				}

				fmt.Println()

				syncCount++
				pulseBuffer = make([]int, 0, 30)
				break
			}
		}
	}

	fmt.Println("Analysis:")
	fmt.Println("  - If we see 9 LONG pulses before the 40-byte pulse, that's the sync")
	fmt.Println("  - If the 40-byte pulse itself IS the sync, it should equal 9 bit-times")
	fmt.Println("  - 9 ALDL bits at 160 baud = 9 × 6250μs = 56,250μs")
	fmt.Println("  - But we're seeing ~8320μs pulses")
	fmt.Println("  - This suggests the 40-byte pulse might be something else (frame marker?)")
}
