package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Testing Edge-Based ALDL Decoder (Arduino-style)\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	fmt.Println("Arduino thresholds:")
	fmt.Println("  Bit 0: 360-370 microseconds")
	fmt.Println("  Bit 1: 1850-1899 microseconds")
	fmt.Println()

	// Show pulse histogram in microseconds
	fmt.Println("Reading 100 pulses and showing classifications:")
	fmt.Println()

	pulseHist := make(map[int]int)
	bitCounts := [2]int{0, 0}

	for i := 0; i < 100; i++ {
		pulseMicros, err := ser.MeasureEdgeTiming()
		if err != nil {
			fmt.Printf("Pulse %3d: ERROR: %v\n", i, err)
			continue
		}

		bit, err := ser.DecodeAldlBitArduino(pulseMicros)
		if err != nil {
			fmt.Printf("Pulse %3d: %4dμs -> ERROR: %v\n", i, pulseMicros, err)
			continue
		}

		bitCounts[bit]++
		pulseHist[pulseMicros]++

		classification := ""
		if pulseMicros >= 360 && pulseMicros <= 370 {
			classification = "✓ Bit 0 (exact match)"
		} else if pulseMicros >= 1850 && pulseMicros <= 1899 {
			classification = "✓ Bit 1 (exact match)"
		} else if bit == 0 {
			classification = "~ Bit 0 (tolerance)"
		} else {
			classification = "~ Bit 1 (tolerance)"
		}

		if i < 20 || pulseMicros >= 360 && pulseMicros <= 370 || pulseMicros >= 1850 && pulseMicros <= 1899 {
			fmt.Printf("Pulse %3d: %4dμs -> bit %d  %s\n", i, pulseMicros, bit, classification)
		}
	}

	fmt.Println()
	fmt.Printf("Summary: %d bit 0s, %d bit 1s\n", bitCounts[0], bitCounts[1])
	fmt.Println()

	// Show histogram
	fmt.Println("Pulse width distribution:")
	for micros := 200; micros <= 2500; micros += 100 {
		count := 0
		for m := micros; m < micros+100; m++ {
			count += pulseHist[m]
		}
		if count > 0 {
			bar := ""
			for j := 0; j < count && j < 50; j++ {
				bar += "█"
			}
			fmt.Printf("%4d-%4dμs: %3d %s\n", micros, micros+99, count, bar)
		}
	}
}
