package errors

import (
	"errors"
	"fmt"
)

// Common error types for the ALDL application
var (
	// ErrSerialPort indicates a serial port communication error
	ErrSerialPort = errors.New("serial port error")

	// ErrIO indicates a general I/O error
	ErrIO = errors.New("I/O error")

	// ErrCSV indicates a CSV writing error
	ErrCSV = errors.New("CSV error")

	// ErrJSON indicates a JSON serialization error
	ErrJSON = errors.New("JSON error")

	// ErrProtocol indicates an ALDL protocol error
	ErrProtocol = errors.New("protocol error")

	// ErrTimeout indicates a communication timeout
	ErrTimeout = errors.New("timeout")

	// ErrInvalidFrame indicates invalid frame data
	ErrInvalidFrame = errors.New("invalid frame")

	// ErrUnsupportedECM indicates an unknown ECM part number
	ErrUnsupportedECM = errors.New("unsupported ECM")

	// ErrConfig indicates a configuration error
	ErrConfig = errors.New("configuration error")
)

// WrapSerialPort wraps an error as a serial port error
func WrapSerialPort(err error, msg string) error {
	return fmt.Errorf("%w: %s: %v", ErrSerialPort, msg, err)
}

// WrapIO wraps an error as an I/O error
func WrapIO(err error, msg string) error {
	return fmt.Errorf("%w: %s: %v", ErrIO, msg, err)
}

// WrapCSV wraps an error as a CSV error
func WrapCSV(err error, msg string) error {
	return fmt.Errorf("%w: %s: %v", ErrCSV, msg, err)
}

// WrapJSON wraps an error as a JSON error
func WrapJSON(err error, msg string) error {
	return fmt.Errorf("%w: %s: %v", ErrJSON, msg, err)
}

// WrapProtocol wraps an error as a protocol error
func WrapProtocol(err error, msg string) error {
	return fmt.Errorf("%w: %s: %v", ErrProtocol, msg, err)
}

// NewTimeout creates a timeout error with a message
func NewTimeout(msg string) error {
	return fmt.Errorf("%w: %s", ErrTimeout, msg)
}

// NewInvalidFrame creates an invalid frame error with a message
func NewInvalidFrame(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalidFrame, msg)
}

// NewUnsupportedECM creates an unsupported ECM error with the part number
func NewUnsupportedECM(partNumber string) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedECM, partNumber)
}

// NewConfig creates a configuration error with a message
func NewConfig(msg string) error {
	return fmt.Errorf("%w: %s", ErrConfig, msg)
}
