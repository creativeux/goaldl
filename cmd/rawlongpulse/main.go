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

	fmt.Printf("Examining RAW bytes during long pulses\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	fmt.Println("Searching for runs of 30+ consecutive 0xFE bytes...")
	fmt.Println()

	buf := make([]byte, 1)
	runLength := 0
	currentByte := byte(0)
	longPulseCount := 0

	for i := 0; i < 5000 && longPulseCount < 2; i++ {
		n, err := port.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		b := buf[0]

		if b == currentByte {
			runLength++
		} else {
			// Run ended
			if currentByte == 0xFE && runLength >= 30 {
				fmt.Printf("=== LONG PULSE #%d: %d consecutive 0xFE bytes ===\n", longPulseCount+1, runLength)
				fmt.Printf("Duration: %dμs (%.2f ALDL bit periods at 160 baud)\n",
					runLength*208, float64(runLength*208)/6250.0)

				// The question: does this represent multiple 1-bits or just one very long 1-bit?
				// At 160 baud, each bit is 6250μs
				// A bit-1 pulse width is 1850-4400μs
				// So this long pulse spans multiple bit periods

				possibleBits := runLength * 208 / 6250
				fmt.Printf("This spans approximately %d ALDL bit periods\n", possibleBits)

				if possibleBits >= 9 {
					fmt.Println("✓ This COULD be 9 consecutive 1-bits!")
				}

				fmt.Println()
				longPulseCount++
			}

			currentByte = b
			runLength = 1
		}
	}

	if longPulseCount == 0 {
		fmt.Println("No long pulses found")
	}

	fmt.Println("Analysis:")
	fmt.Println("  If a 40-byte pulse spans ~1.3 ALDL bit periods, it's just one long bit")
	fmt.Println("  If it spans ~9 bit periods, it's the sync (9 consecutive 1s)")
	fmt.Println("  The key is: how does the ECM encode 9 consecutive 1s over PWM?")
}
