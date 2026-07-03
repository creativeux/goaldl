package main

import (
	"fmt"
	"log"
	"time"

	"go.bug.st/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 8192

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

	fmt.Printf("Testing 8192 baud (linuxaldl rate)\n")
	fmt.Printf("At 8192 baud: ~51x oversampling of 160 baud ALDL\n\n")

	// Read raw bytes and look for patterns
	buf := make([]byte, 1)
	fmt.Println("Raw byte stream (first 100 bytes):")

	byteHist := make(map[byte]int)

	for i := 0; i < 100; i++ {
		n, err := port.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		b := buf[0]
		byteHist[b]++

		if i > 0 && i%20 == 0 {
			fmt.Println()
		}
		fmt.Printf("%02X ", b)
	}
	fmt.Println()
	fmt.Println()

	fmt.Println("Byte distribution:")
	for b := byte(0); b <= 255; b++ {
		if byteHist[b] > 0 {
			ones := 0
			for j := 0; j < 8; j++ {
				if (b & (1 << j)) != 0 {
					ones++
				}
			}
			fmt.Printf("  0x%02X (%08b, %d ones): %d occurrences\n", b, b, ones, byteHist[b])
		}
	}
}
