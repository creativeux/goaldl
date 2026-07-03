package main

import (
	"fmt"
	"log"
	"time"

	"go.bug.st/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer port.Close()

	if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
		log.Fatalf("Failed to set timeout: %v", err)
	}

	fmt.Printf("Looking for sync character as long runs of 0xFE bytes\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)
	fmt.Println("Sync character (9 ALDL bit-times) = 9 × 6250μs = 56,250μs")
	fmt.Println("At 4800 baud: 56,250μs / 208μs = ~270 bytes of 0xFE")
	fmt.Println("But could appear as shorter runs if transmitted differently")
	fmt.Println()

	buf := make([]byte, 1)
	currentByte := byte(0)
	runLength := 0
	maxRun := 0
	longRuns := make(map[int]int) // track runs >= 20 bytes

	for i := 0; i < 2000; i++ {
		n, err := port.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		b := buf[0]

		if b == currentByte {
			runLength++
			if b == 0xFE && runLength > maxRun {
				maxRun = runLength
			}
		} else {
			// Run ended
			if currentByte == 0xFE && runLength >= 20 {
				duration := runLength * 208
				fmt.Printf("Long HIGH pulse: %3d bytes (≈ %5dμs)\n", runLength, duration)
				longRuns[runLength]++
			}
			currentByte = b
			runLength = 1
		}
	}

	fmt.Printf("\nLongest run of 0xFE bytes: %d (≈ %dμs)\n", maxRun, maxRun*208)

	if maxRun >= 200 {
		fmt.Println("✓ This could be the sync character!")
	} else if maxRun >= 30 {
		fmt.Println("⚠️  Seeing long pulses but not quite sync length")
		fmt.Println("    Could be sync transmitted differently, or need different baud rate")
	} else {
		fmt.Println("❌ Not seeing sync-length pulses")
		fmt.Println("    Possibilities:")
		fmt.Println("    1. Sync is encoded differently")
		fmt.Println("    2. Wrong baud rate")
		fmt.Println("    3. Need to look at raw bit-level stream")
	}
}
