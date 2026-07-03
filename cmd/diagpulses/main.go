package main

import (
	"fmt"
	"log"

	"goaldl/pkg/serial"
)

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"
	baudRate := 4800

	fmt.Printf("Detailed Pulse Diagnostics\n")
	fmt.Printf("Port: %s at %d baud\n\n", portName, baudRate)

	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		log.Fatalf("Failed to open port: %v", err)
	}
	defer ser.Close()

	fmt.Println("Reading 200 pulses with full details:")
	fmt.Println("(Showing all pulses to find patterns)")
	fmt.Println()

	for i := 0; i < 200; i++ {
		pulseMicros, err := ser.MeasureEdgeTiming()
		if err != nil {
			fmt.Printf("%3d: ERROR: %v\n", i, err)
			continue
		}

		bit, err := ser.DecodeAldlBitArduino(pulseMicros)

		status := ""
		if err != nil {
			status = fmt.Sprintf("SKIP (%v)", err)
		} else {
			status = fmt.Sprintf("-> bit %d", bit)
		}

		fmt.Printf("%3d: %4dμs %s\n", i, pulseMicros, status)
	}
}
