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

	if err := port.SetReadTimeout(50 * time.Millisecond); err != nil {
		log.Fatalf("Failed to set timeout: %v", err)
	}

	fmt.Printf("Searching for VERY long runs of 0xFE (sync should be ~86-270 bytes)\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	fmt.Println("Expected sync lengths:")
	fmt.Println("  - If 9 bits × full period (6250μs): 56,250μs = 270 bytes")
	fmt.Println("  - If 9 bits × pulse width (~2000μs): 18,000μs = 86 bytes")
	fmt.Println()
	fmt.Println("Histogram of 0xFE run lengths:")
	fmt.Println()

	buf := make([]byte, 1)
	runLengths := make(map[int]int)
	runLength := 0
	currentByte := byte(0)
	maxRun := 0

	for i := 0; i < 10000; i++ {
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
			if currentByte == 0xFE && runLength > 0 {
				runLengths[runLength]++
			}
			currentByte = b
			runLength = 1
		}
	}

	// Show distribution
	for length := 1; length <= maxRun; length++ {
		if runLengths[length] > 0 {
			duration := length * 208
			bar := ""
			barLen := runLengths[length]
			if barLen > 50 {
				barLen = 50
			}
			for j := 0; j < barLen; j++ {
				bar += "█"
			}

			special := ""
			if length >= 80 {
				special = " ← POSSIBLE SYNC!"
			}

			fmt.Printf("%3d bytes (%5dμs): %4d occurrences %s%s\n",
				length, duration, runLengths[length], bar, special)
		}
	}

	fmt.Printf("\nMaximum run length: %d bytes (%dμs)\n", maxRun, maxRun*208)

	if maxRun < 80 {
		fmt.Println("\n❌ Never saw a run long enough to be the sync character")
		fmt.Println("This suggests:")
		fmt.Println("  1. Sync is not being transmitted as one continuous pulse")
		fmt.Println("  2. Sync may have gaps/breaks in it")
		fmt.Println("  3. We need to look at bit-level patterns differently")
	}
}
