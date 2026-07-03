package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Showing decoded frames from %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	// Read 5 frames (20 bytes each)
	for frameNum := 0; frameNum < 5; frameNum++ {
		frame := make([]byte, 20)

		for byteIdx := 0; byteIdx < 20; byteIdx++ {
			var b byte

			// Read 8 bits MSB first
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

		fmt.Printf("Frame %d: ", frameNum)
		for i, b := range frame {
			if i == 1 || i == 2 {
				fmt.Printf("[%02X] ", b)
			} else {
				fmt.Printf("%02X ", b)
			}
		}

		// Check if this matches expected PROM ID
		if frame[1] == 24 && frame[2] == 147 {
			fmt.Print(" ✓ PROM ID MATCH!")
		} else {
			fmt.Printf(" (expected PROM: [18] [93], got [%02X] [%02X])", frame[1], frame[2])
		}
		fmt.Println()
	}
}
