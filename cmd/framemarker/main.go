package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Looking for frame markers (very long pulses) and reading data after them\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	fmt.Println("Searching for long pulses (30+ bytes) that might mark frame boundaries...")
	fmt.Println()

	frameCount := 0

	for frameCount < 5 {
		// Look for a very long pulse (potential frame marker)
		for {
			runLength, err := ser.MeasurePulseWidthRunLength()
			if err != nil {
				continue
			}

			if runLength >= 30 {
				fmt.Printf("Found long pulse: %d bytes (%dμs) - treating as frame marker\n",
					runLength, runLength*208)

				// Read next 20 bytes (160 bits) as a frame
				frame := make([]byte, 20)
				for i := 0; i < 20; i++ {
					var b byte

					// Read 8 bits MSB first
					for bitIdx := 7; bitIdx >= 0; bitIdx-- {
						bit, err := ser.ReadAldlBitRunLength()
						if err != nil {
							fmt.Printf("Error reading bit: %v\n", err)
							goto nextFrame
						}

						if bit == 1 {
							b |= (1 << bitIdx)
						}
					}
					frame[i] = b
				}

				fmt.Printf("Frame %d: ", frameCount)
				for i, b := range frame {
					if i == 1 || i == 2 {
						fmt.Printf("[%02X] ", b)
					} else {
						fmt.Printf("%02X ", b)
					}
				}

				if frame[1] == 24 && frame[2] == 147 {
					fmt.Print(" ✓✓✓ PROM ID MATCH!")
				} else {
					fmt.Printf(" (expected [18] [93], got [%02X] [%02X])", frame[1], frame[2])
				}
				fmt.Println()

				frameCount++
				break
			}
		}
	nextFrame:
	}

	fmt.Println()
	fmt.Println("Expected PROM ID: [24, 147] = [0x18, 0x93]")
}
