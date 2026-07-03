package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Testing 9-bit ALDL decoder\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	// Create 9-bit reader
	reader := serial.NewAldlReader9Bit(ser)

	fmt.Println("Searching for sync pattern (9 consecutive 1s = 0x1FF)...")
	err = reader.FindSync()
	if err != nil {
		log.Fatalf("Failed to find sync: %v", err)
	}

	fmt.Println("✓ Sync pattern found!")
	fmt.Println()

	// Read 5 frames
	fmt.Println("Reading frames:")
	for frameNum := 0; frameNum < 5; frameNum++ {
		frame, err := reader.ReadFrame()
		if err != nil {
			fmt.Printf("Frame %d: ERROR: %v\n", frameNum, err)

			// Try to re-sync
			fmt.Println("  Attempting to re-sync...")
			err = reader.FindSync()
			if err != nil {
				log.Fatalf("  Failed to re-sync: %v", err)
			}
			fmt.Println("  ✓ Re-synced, continuing...")
			continue
		}

		fmt.Printf("Frame %d: ", frameNum)
		for i, b := range frame {
			if i == 1 || i == 2 {
				fmt.Printf("[%02X] ", b)
			} else {
				fmt.Printf("%02X ", b)
			}
		}

		// Check PROM ID
		if frame[1] == 24 && frame[2] == 147 {
			fmt.Print(" ✓✓✓ PROM ID MATCH!")
		} else {
			fmt.Printf(" (expected [18] [93], got [%02X] [%02X])", frame[1], frame[2])
		}
		fmt.Println()
	}

	fmt.Println()
	fmt.Println("Expected PROM ID: [24, 147] = [0x18, 0x93]")
}
