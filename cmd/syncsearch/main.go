package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("SYNC SEARCH: Looking ONLY for 9 consecutive long pulses\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	fmt.Println("Reading pulses and filtering out 1-byte glitches...")
	fmt.Println("Looking for 9 consecutive 'long' pulses (ignoring glitches)")
	fmt.Println()

	// Read pulses, skip glitches, classify as short/long
	consecutiveLongs := 0
	maxConsecutiveLongs := 0
	totalPulses := 0

	for i := 0; i < 500; i++ {
		runLength, err := ser.MeasurePulseWidthRunLength()
		if err != nil {
			continue
		}

		// Skip 1-byte glitches entirely
		if runLength == 1 {
			continue
		}

		totalPulses++

		// Classify pulse
		var pulseType string
		var isLong bool

		if runLength >= 2 && runLength <= 4 {
			pulseType = "SHORT"
			isLong = false
		} else if runLength >= 5 {
			pulseType = "LONG"
			isLong = true
		}

		if isLong {
			consecutiveLongs++
			if consecutiveLongs > maxConsecutiveLongs {
				maxConsecutiveLongs = consecutiveLongs
			}

			if consecutiveLongs == 9 {
				fmt.Printf("✓✓✓ FOUND SYNC! 9 consecutive long pulses at pulse #%d\n", totalPulses)
				fmt.Printf("    This is the sync character (0x1FF)\n")
				fmt.Println()

				// Read next 20 bytes after sync
				fmt.Println("Reading frame immediately after sync:")
				frame := make([]byte, 20)
				for byteIdx := 0; byteIdx < 20; byteIdx++ {
					var b byte
					for bitIdx := 7; bitIdx >= 0; bitIdx-- {
						bit, err := ser.ReadAldlBitRunLength()
						if err != nil {
							fmt.Printf("Error: %v\n", err)
							return
						}
						if bit == 1 {
							b |= (1 << bitIdx)
						}
					}
					frame[byteIdx] = b
				}

				fmt.Print("Frame: ")
				for i, b := range frame {
					if i == 1 || i == 2 {
						fmt.Printf("[%02X] ", b)
					} else {
						fmt.Printf("%02X ", b)
					}
				}

				if frame[1] == 24 && frame[2] == 147 {
					fmt.Print(" ✓✓✓ PROM ID MATCH!")
				}
				fmt.Println()
				return
			}

			if consecutiveLongs <= 20 && consecutiveLongs > 4 {
				fmt.Printf("Pulse %4d: %4s (%2d bytes) - %d consecutive longs so far\n",
					totalPulses, pulseType, runLength, consecutiveLongs)
			}
		} else {
			// Short pulse breaks the run
			if consecutiveLongs >= 4 {
				fmt.Printf("  Run of %d longs ended by SHORT pulse\n", consecutiveLongs)
			}
			consecutiveLongs = 0
		}
	}

	fmt.Println()
	fmt.Printf("❌ Sync NOT found in %d pulses\n", totalPulses)
	fmt.Printf("Maximum consecutive long pulses seen: %d (need 9)\n", maxConsecutiveLongs)
	fmt.Println()
	fmt.Println("This means:")
	fmt.Println("  1. We're never seeing the sync character in the stream")
	fmt.Println("  2. ECM might only send sync at startup")
	fmt.Println("  3. We might need to send a command to ECM first")
	fmt.Println("  4. Threshold might be wrong (miscategorizing long pulses as short)")
}
