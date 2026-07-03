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

	fmt.Printf("Analyzing pulse patterns from %s at %d baud...\n\n", portName, baudRate)
	fmt.Println("Looking for pulse sequences (runs of 0xFE = HIGH, 0x00 = LOW)")
	fmt.Println()

	buf := make([]byte, 1)
	var currentByte byte
	var runLength int
	var pulseCount int

	for i := 0; i < 500; i++ {
		n, err := port.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		b := buf[0]

		// Track runs of similar bytes
		if i == 0 {
			currentByte = b
			runLength = 1
		} else if b == currentByte {
			runLength++
		} else {
			// Run ended, print it
			if runLength > 0 {
				state := "LOW"
				if currentByte == 0xFE {
					state = "HIGH"
				}

				// At 4800 baud, each byte is ~208μs
				// ALDL logic 0: 360-370μs ≈ 2 bytes
				// ALDL logic 1: 1850-4400μs ≈ 9-21 bytes
				duration := runLength * 208 // microseconds

				aldlBit := "?"
				if state == "HIGH" {
					if duration >= 300 && duration <= 500 {
						aldlBit = "0 (short pulse)"
					} else if duration >= 1700 && duration <= 4500 {
						aldlBit = "1 (long pulse)"
					}
				}

				if state == "HIGH" {  // Only print HIGH pulses (actual data)
					fmt.Printf("Pulse %3d: %4s x%2d bytes ≈ %4dμs -> ALDL bit %s\n",
						pulseCount, state, runLength, duration, aldlBit)
					pulseCount++
				}
			}
			currentByte = b
			runLength = 1
		}
	}
}
