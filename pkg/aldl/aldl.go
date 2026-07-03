package aldl

import (
	"fmt"
	"time"

	"goaldl/pkg/errors"
	"goaldl/pkg/serial"
)

const (
	// ALDL protocol constants
	aldlSyncWord       = 0x1FF // 9-bit sync character (9 consecutive 1s)
	aldlBitTimeUs      = 6250  // 160 baud bit time in microseconds
	aldlZeroPulseMin   = 360   // Logic 0 pulse width minimum (microseconds)
	aldlZeroPulseMax   = 370   // Logic 0 pulse width maximum (microseconds)
	aldlOnePulseMin    = 1850  // Logic 1 pulse width minimum (microseconds)
	aldlOnePulseMax    = 1899  // Logic 1 pulse width maximum (microseconds)
	aldlDefaultFrameSize = 20   // Default frame size in bytes
	aldlExtendedFrameSize = 25  // Extended frame size (with BLM data)

	// Sync timeout
	syncTimeoutSeconds = 15
)

// Frame represents a single ALDL data frame
type Frame struct {
	Data      []byte
	Timestamp time.Time
}

// Protocol handles ALDL protocol communication
type Protocol struct {
	serial    *serial.AldlSerial
	frameSize int
}

// New creates a new ALDL protocol handler
func New(portName string) (*Protocol, error) {
	ser, err := serial.New(portName)
	if err != nil {
		return nil, err
	}

	return &Protocol{
		serial:    ser,
		frameSize: aldlDefaultFrameSize,
	}, nil
}

// Close closes the serial port
func (p *Protocol) Close() error {
	return p.serial.Close()
}

// SetFrameSize sets the expected frame size (20 or 25 bytes)
func (p *Protocol) SetFrameSize(size int) error {
	if size != aldlDefaultFrameSize && size != aldlExtendedFrameSize {
		return errors.NewConfig(fmt.Sprintf("invalid frame size: %d (must be 20 or 25)", size))
	}
	p.frameSize = size
	return nil
}

// WaitForSync attempts to synchronize with ALDL data stream
//
// ⚠️ POTENTIAL ISSUE AREA FOR GIBBERISH DATA:
// The sync detection uses pattern-based matching rather than strict PWM timing.
// It looks for expected PROM ID values [24, 147] and validates sensor ranges.
//
// Possible issues:
// 1. Pattern matching may lock onto wrong data that happens to look valid
// 2. The expected PROM ID may be different for different ECMs
// 3. Sensor range validation may not be strict enough
// 4. May sync to the middle of a frame instead of the start
// 5. The "force resync" strategy (reading full frame after each frame) may cause drift
//
// The Rust code mentions: "Pattern-based detection looking for expected PROM ID
// [24, 147] and realistic sensor values - more reliable than timing-based sync"
// However, if the data is gibberish, this assumption may be wrong.
func (p *Protocol) WaitForSync() error {
	return p.WaitForSyncWithVerbose(false)
}

// WaitForSyncWithVerbose attempts to synchronize with ALDL data stream
// If verbose is true, prints diagnostic information about frames being read
func (p *Protocol) WaitForSyncWithVerbose(verbose bool) error {
	timeout := time.After(syncTimeoutSeconds * time.Second)
	attemptCount := 0

	for {
		select {
		case <-timeout:
			return errors.NewTimeout("sync timeout after 15 seconds")
		default:
			// Try to read a frame
			frame, err := p.readFrameInternal()
			if err != nil {
				continue // Keep trying
			}

			attemptCount++

			// Validate expected patterns
			// Looking for PROM ID [24, 147] at bytes 1-2 (byte 0 is MW2/Mode Word)
			if len(frame) >= 3 && frame[1] == 24 && frame[2] == 147 {
				// Found expected PROM ID pattern
				if verbose {
					fmt.Printf("✓ Synced after %d attempts\n", attemptCount)
				}
				return nil
			}

			// Diagnostic output for debugging sync issues
			if verbose && attemptCount <= 20 {
				fmt.Printf("Attempt %d: [%d, %d, %d] (expected MW2=?, PROMIDA=24, PROMIDB=147) - ",
					attemptCount, frame[0], frame[1], frame[2])
				// Show first 6 bytes in hex
				fmt.Printf("First bytes: ")
				for i := 0; i < 6 && i < len(frame); i++ {
					fmt.Printf("%02X ", frame[i])
				}
				fmt.Println()
			}
		}
	}
}

// ReadFrame reads a single ALDL frame with timestamp
func (p *Protocol) ReadFrame() (*Frame, error) {
	data, err := p.readFrameInternal()
	if err != nil {
		return nil, err
	}

	return &Frame{
		Data:      data,
		Timestamp: time.Now(),
	}, nil
}

// readFrameInternal reads frame data without creating a Frame struct
func (p *Protocol) readFrameInternal() ([]byte, error) {
	frame := make([]byte, p.frameSize)

	for i := 0; i < p.frameSize; i++ {
		b, err := p.readAldlByte()
		if err != nil {
			return nil, err
		}
		frame[i] = b
	}

	return frame, nil
}

// readAldlByte reads one ALDL byte (8 bits, MSB first)
//
// ⚠️ POTENTIAL ISSUE AREA FOR GIBBERISH DATA:
// ALDL uses MSB-first bit ordering (most significant bit first).
// If the bit ordering is wrong, all data will be scrambled.
//
// Current implementation:
// - Reads 8 bits
// - Shifts each bit into position (bit 7 down to bit 0)
// - MSB is read first and placed in the high bit
//
// Things to check if data is wrong:
// 1. Is the bit ordering actually MSB-first or LSB-first?
// 2. Are we shifting in the right direction?
// 3. Should we be reversing the bit order?
func (p *Protocol) readAldlByte() (byte, error) {
	var result byte

	// Read 8 bits, MSB first
	for i := 7; i >= 0; i-- {
		bit, err := p.serial.ReadAldlBit()
		if err != nil {
			return 0, err
		}

		// Place bit in position
		if bit == 1 {
			result |= (1 << i)
		}
	}

	return result, nil
}

// ContinuousRead returns a channel that yields frames continuously
//
// ⚠️ POTENTIAL ISSUE AREA:
// After reading each frame, this forces a resync by calling WaitForSync.
// This is intended to maintain frame alignment, but may cause:
// 1. Unnecessary delays between frames
// 2. Loss of data if resync takes too long
// 3. Drift if the sync pattern appears within valid data
//
// The Rust version does this too, so this may be inherent to the design.
// However, if the protocol is well-defined, we shouldn't need to resync
// after every single frame.
func (p *Protocol) ContinuousRead() <-chan *Frame {
	ch := make(chan *Frame)

	go func() {
		defer close(ch)

		for {
			// Wait for sync before each frame
			// ⚠️ This may be excessive - consider reading frames without resyncing
			if err := p.WaitForSync(); err != nil {
				return
			}

			frame, err := p.ReadFrame()
			if err != nil {
				return
			}

			ch <- frame
		}
	}()

	return ch
}

// ReadAldlBit is a convenience wrapper for serial.ReadAldlBit
func (p *Protocol) ReadAldlBit() (byte, error) {
	return p.serial.ReadAldlBit()
}
