// Package serial is a thin cross-platform wrapper over go.bug.st/serial for
// ALDL use: open a port at a given baud, read raw bytes, flush, and list USB
// serial devices. All ALDL decoding lives in pkg/decoder — this package deals
// only in raw bytes.
package serial

import (
	"strings"
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
// across Linux (/dev/ttyUSB*, /dev/ttyACM*), Windows (COM*), macOS
// (/dev/cu.* for common chipsets), and the BSDs (/dev/cuaU*).
func AvailablePorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, errors.WrapSerialPort(err, "failed to list ports")
	}
	return filterUSBPorts(ports), nil
}

// macOS /dev/cu.<name> prefixes for common USB-serial chipsets.
var cuUSBPrefixes = []string{
	"PL2303-",      // Prolific PL2303 (vendor driver)
	"usbserial",    // Prolific PL2303 (DriverKit app), FTDI
	"SLAB",         // Silicon Labs CP210x
	"wchusbserial", // WCH CH340/CH341
	"usbmodem",     // CDC-ACM devices
}

// filterUSBPorts keeps the port names that look like USB-to-serial adapters.
func filterUSBPorts(ports []string) []string {
	var usbPorts []string
	for _, port := range ports {
		isUSB := strings.HasPrefix(port, "/dev/ttyUSB") || // Linux USB-serial
			strings.HasPrefix(port, "/dev/ttyACM") || // Linux CDC-ACM
			strings.HasPrefix(port, "COM") || // Windows
			strings.HasPrefix(port, "/dev/cuaU") // FreeBSD/OpenBSD USB-serial

		if name, ok := strings.CutPrefix(port, "/dev/cu."); ok {
			for _, prefix := range cuUSBPrefixes {
				if strings.HasPrefix(name, prefix) {
					isUSB = true
					break
				}
			}
		}

		if isUSB {
			usbPorts = append(usbPorts, port)
		}
	}
	return usbPorts
}
