package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Searching for VERY LONG pulses (potential sync)\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	fmt.Println("Sync character = 9 consecutive bit-1 pulses")
	fmt.Println("If each bit-1 is ~1872μs, then 9 × 1872 = 16,848μs")
	fmt.Println("Or if sync is one continuous LOW, then 9 × 6250μs = 56,250μs")
	fmt.Println()
	fmt.Println("Searching for pulses > 10000μs:")
	fmt.Println()

	maxPulse := 0
	longPulseCount := 0

	for i := 0; i < 500; i++ {
		pulseMicros, err := ser.MeasureEdgeTiming()
		if err != nil {
			continue
		}

		if pulseMicros > maxPulse {
			maxPulse = pulseMicros
		}

		if pulseMicros > 10000 {
			longPulseCount++
			fmt.Printf("Pulse %3d: %5dμs ← VERY LONG! Could be sync?\n", i, pulseMicros)
		}
	}

	fmt.Println()
	fmt.Printf("Longest pulse seen: %dμs\n", maxPulse)
	fmt.Printf("Pulses > 10000μs: %d\n", longPulseCount)

	if maxPulse < 10000 {
		fmt.Println("\n⚠️ Never saw a pulse long enough to be the full sync character")
		fmt.Println("This suggests sync might be transmitted as 9 separate bit-1 pulses")
		fmt.Println("But we're only seeing max 2 consecutive bit-1s...")
	}
}
