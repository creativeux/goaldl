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

	fmt.Printf("Reading 100 bytes from %s at %d baud...\n\n", portName, baudRate)

	buf := make([]byte, 1)
	for i := 0; i < 100; i++ {
		n, err := port.Read(buf)
		if err != nil {
			fmt.Printf("%3d: ERROR: %v\n", i, err)
			continue
		}
		if n == 0 {
			fmt.Printf("%3d: TIMEOUT (no data)\n", i)
			continue
		}

		b := buf[0]
		ones := 0
		for j := 0; j < 8; j++ {
			if (b & (1 << j)) != 0 {
				ones++
			}
		}

		fmt.Printf("%3d: 0x%02X = %08b  (%d ones)\n", i, b, b, ones)
	}
}