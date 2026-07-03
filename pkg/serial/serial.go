// Package serial is a thin cross-platform wrapper over go.bug.st/serial for
// ALDL use: open a port at a given baud, read raw bytes, flush, and list USB
// serial devices. All ALDL decoding lives in pkg/decoder — this package deals
// only in raw bytes.
package serial

import (
	"time"

	"go.bug.st/serial"
	"goaldl/pkg/errors"
)

const (
	// Serial line configuration for ALDL sampling.
	dataBits  = 8
	stopBits  = serial.OneStopBit
	parity    = serial.NoParity
	timeoutMs = 50
)

// AldlSerial wraps a serial port opened for ALDL capture.
type AldlSerial struct {
	port     serial.Port
	baudRate int
}

// NewWithBaudRate opens a serial port at the given UART sampling baud rate.
func NewWithBaudRate(portName string, baudRate int) (*AldlSerial, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: dataBits,
		StopBits: stopBits,
		Parity:   parity,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, errors.WrapSerialPort(err, "failed to open port")
	}
	if err := port.SetReadTimeout(timeoutMs * time.Millisecond); err != nil {
		port.Close()
		return nil, errors.WrapSerialPort(err, "failed to set timeout")
	}
	return &AldlSerial{port: port, baudRate: baudRate}, nil
}

// Close closes the serial port.
func (s *AldlSerial) Close() error {
	if s.port != nil {
		return s.port.Close()
	}
	return nil
}

// Read reads up to len(buf) bytes, returning however many arrived before the
// read timeout (possibly 0). Partial reads are expected for a live stream.
func (s *AldlSerial) Read(buf []byte) (int, error) {
	return s.port.Read(buf)
}

// ResetInputBuffer discards any stale bytes buffered by the driver, so a
// capture starts with live data.
func (s *AldlSerial) ResetInputBuffer() error {
	return s.port.ResetInputBuffer()
}

// AvailablePorts lists serial ports that look like USB-to-serial adapters,
// across Linux (/dev/ttyUSB*, /dev/ttyACM*), Windows (COM*), and macOS
// (/dev/cu.* for common chipsets).
func AvailablePorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, errors.WrapSerialPort(err, "failed to list ports")
	}

	var usbPorts []string
	for _, port := range ports {
		isUSB := false

		if len(port) >= 11 && (port[:11] == "/dev/ttyUSB" || port[:11] == "/dev/ttyACM") {
			isUSB = true
		}
		if len(port) >= 3 && port[:3] == "COM" {
			isUSB = true
		}
		if len(port) >= 8 && port[:8] == "/dev/cu." {
			name := port[8:]
			if len(name) >= 7 && name[:7] == "PL2303-" ||
				len(name) >= 9 && name[:9] == "usbserial" ||
				len(name) >= 4 && name[:4] == "SLAB" || // Silicon Labs CP210x
				len(name) >= 5 && name[:5] == "wchusbserial" || // CH340
				len(name) >= 8 && name[:8] == "usbmodem" {
				isUSB = true
			}
		}

		if isUSB {
			usbPorts = append(usbPorts, port)
		}
	}
	return usbPorts, nil
}
