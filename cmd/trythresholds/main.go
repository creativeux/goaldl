package main

import (
	"fmt"
	"log"
	"time"

	"go.bug.st/serial"
	"goaldl/pkg/errors"
)

// TestSerial is a test version with configurable threshold
type TestSerial struct {
	port      serial.Port
	threshold int // bytes: <= threshold = bit 0, > threshold = bit 1
}

func NewTestSerial(portName string, baudRate int) (*TestSerial, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, err
	}

	if err := port.SetReadTimeout(50 * time.Millisecond); err != nil {
		port.Close()
		return nil, err
	}

	return &TestSerial{port: port, threshold: 4}, nil
}

func (s *TestSerial) Close() error {
	return s.port.Close()
}

func (s *TestSerial) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	n, err := s.port.Read(buf)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, errors.NewTimeout("no data")
	}
	return buf[0], nil
}

func (s *TestSerial) MeasurePulseWidth() (int, error) {
	const highByte = 0xFE
	const maxSkip = 50

	for i := 0; i < maxSkip; i++ {
		b, err := s.ReadByte()
		if err != nil {
			return 0, err
		}
		if b == highByte {
			runLength := 1
			for {
				b, err := s.ReadByte()
				if err != nil {
					return 0, err
				}
				if b == highByte {
					runLength++
				} else {
					return runLength, nil
				}
				if runLength > 100 {
					return runLength, nil
				}
			}
		}
	}
	return 0, errors.NewTimeout("no pulse")
}

func (s *TestSerial) ReadBit() (byte, error) {
	for i := 0; i < 10; i++ {
		runLength, err := s.MeasurePulseWidth()
		if err != nil {
			return 0, err
		}

		if runLength == 1 {
			continue // skip glitches
		}

		if runLength <= s.threshold {
			return 0, nil
		}
		return 1, nil
	}
	return 0, errors.NewTimeout("too many glitches")
}

func testThreshold(portName string, threshold int) {
	ser, err := NewTestSerial(portName, 4800)
	if err != nil {
		log.Printf("Failed to open port: %v", err)
		return
	}
	defer ser.Close()

	ser.threshold = threshold

	// Find a long pulse
	for {
		runLength, err := ser.MeasurePulseWidth()
		if err != nil {
			continue
		}

		if runLength >= 30 {
			// Read frame
			frame := make([]byte, 20)
			for i := 0; i < 20; i++ {
				var b byte
				for bitIdx := 7; bitIdx >= 0; bitIdx-- {
					bit, err := ser.ReadBit()
					if err != nil {
						return
					}
					if bit == 1 {
						b |= (1 << bitIdx)
					}
				}
				frame[i] = b
			}

			fmt.Printf("Threshold %d: ", threshold)
			for i := 0; i < 5; i++ {
				if i == 1 || i == 2 {
					fmt.Printf("[%02X] ", frame[i])
				} else {
					fmt.Printf("%02X ", frame[i])
				}
			}
			if frame[1] == 24 && frame[2] == 147 {
				fmt.Print("✓✓✓ MATCH!")
			}
			fmt.Println()
			return
		}
	}
}

func main() {
	portName := "/dev/cu.PL2303-USBtoUART110"

	fmt.Println("Testing different thresholds to find correct PROM ID [18] [93]")
	fmt.Println()

	for threshold := 2; threshold <= 10; threshold++ {
		testThreshold(portName, threshold)
		time.Sleep(100 * time.Millisecond)
	}
}
