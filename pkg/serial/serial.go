package serial

import (
	"fmt"
	"time"

	"go.bug.st/serial"
	"goaldl/pkg/errors"
)

const (
	// ALDL uses 160 baud logical rate with PWM encoding
	// Default sampling at 2400 baud (15x oversampling) to decode PWM pulses
	defaultBaudRate = 2400

	// Serial configuration
	dataBits  = 8
	stopBits  = serial.OneStopBit
	parity    = serial.NoParity
	timeoutMs = 50
)

// AldlSerial wraps a serial port for ALDL communication
type AldlSerial struct {
	port     serial.Port
	baudRate int
}

// New opens a serial port configured for ALDL communication at default baud rate
func New(portName string) (*AldlSerial, error) {
	return NewWithBaudRate(portName, defaultBaudRate)
}

// NewWithBaudRate opens a serial port configured for ALDL communication at specified baud rate
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

	// Set read timeout
	if err := port.SetReadTimeout(timeoutMs * time.Millisecond); err != nil {
		port.Close()
		return nil, errors.WrapSerialPort(err, "failed to set timeout")
	}

	return &AldlSerial{port: port, baudRate: baudRate}, nil
}

// Close closes the serial port
func (s *AldlSerial) Close() error {
	if s.port != nil {
		return s.port.Close()
	}
	return nil
}

// Read reads up to len(buf) bytes, returning however many arrived before the
// read timeout (possibly 0). Use for raw capture where partial reads are fine.
func (s *AldlSerial) Read(buf []byte) (int, error) {
	return s.port.Read(buf)
}

// ResetInputBuffer discards any stale bytes buffered by the driver, so a
// capture starts with live data.
func (s *AldlSerial) ResetInputBuffer() error {
	return s.port.ResetInputBuffer()
}

// ReadByte reads a single byte from the serial port
func (s *AldlSerial) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	n, err := s.port.Read(buf)
	if err != nil {
		return 0, errors.WrapSerialPort(err, "failed to read byte")
	}
	if n == 0 {
		return 0, errors.NewTimeout("no data available")
	}
	return buf[0], nil
}

// ReadBytes reads multiple bytes from the serial port
func (s *AldlSerial) ReadBytes(count int) ([]byte, error) {
	buf := make([]byte, count)
	n, err := s.port.Read(buf)
	if err != nil {
		return nil, errors.WrapSerialPort(err, "failed to read bytes")
	}
	if n < count {
		return nil, errors.NewTimeout(fmt.Sprintf("expected %d bytes, got %d", count, n))
	}
	return buf, nil
}

// MeasurePulseWidth measures the pulse width of an ALDL bit by reading a UART byte
// and counting the number of '1' bits in it
//
// ⚠️ POTENTIAL ISSUE AREA FOR GIBBERISH DATA:
// This function decodes ALDL bits by counting '1' bits in the UART byte.
// The ALDL protocol uses PWM encoding at 160 baud, and we sample at 2400 baud (15x).
// - Logic 0: Short pulse (360-370μs) -> fewer '1' bits in UART byte
// - Logic 1: Long pulse (1850-1899μs) -> more '1' bits in UART byte
//
// The threshold logic may need adjustment:
// - Current: 0-2 ones = 0, 4-8 ones = 1, 3 ones = error
// - This may be too strict or may not account for timing variations
func (s *AldlSerial) MeasurePulseWidth() (int, error) {
	b, err := s.ReadByte()
	if err != nil {
		return 0, err
	}

	// Count the number of '1' bits in the byte
	count := 0
	for i := 0; i < 8; i++ {
		if (b & (1 << i)) != 0 {
			count++
		}
	}

	return count, nil
}

// ReadAldlBit reads and decodes a single ALDL bit
//
// ⚠️ POTENTIAL ISSUE AREA FOR GIBBERISH DATA:
// The bit decoding logic here may be causing incorrect bit values.
// Current threshold:
// - 0-2 ones -> ALDL bit 0
// - 4-8 ones -> ALDL bit 1
// - 3 ones -> error (transitional state)
//
// Possible issues:
// 1. The threshold may not be appropriate for actual signal characteristics
// 2. Timing variations may cause misclassification
// 3. The 3-ones case may be too aggressive (could be valid data)
// 4. Serial buffer timing may not align with ALDL bit boundaries
func (s *AldlSerial) ReadAldlBit() (byte, error) {
	ones, err := s.MeasurePulseWidth()
	if err != nil {
		return 0, err
	}

	return s.DecodeAldlBit(ones)
}

// DecodeAldlBit decodes an ALDL bit value from the count of '1' bits
//
// ⚠️ CRITICAL AREA FOR DEBUGGING:
// This is the core logic that translates UART sampling into ALDL bits.
// If data is gibberish, this is a prime suspect.
//
// Logic:
// - 0-2 ones: Interpret as ALDL bit 0 (short pulse)
// - 3 ones: Error/transitional state
// - 4-8 ones: Interpret as ALDL bit 1 (long pulse)
//
// Things to investigate if data is wrong:
// 1. Are the thresholds correct? (maybe 3 should map to 0 or 1?)
// 2. Is the baud rate relationship correct? (2400 vs 160)
// 3. Are we sampling at the right points in the waveform?
// 4. Should we use different thresholds based on signal quality?
func (s *AldlSerial) DecodeAldlBit(ones int) (byte, error) {
	switch {
	case ones <= 2:
		return 0, nil
	case ones >= 4 && ones <= 8:
		return 1, nil
	default:
		// 3 ones is ambiguous - this might be too strict
		return 0, errors.NewInvalidFrame(fmt.Sprintf("ambiguous bit with %d ones", ones))
	}
}

// ReadAldlBitEdgeBased reads and decodes a single ALDL bit using edge-based timing
//
// This mimics the Arduino approach: measure microseconds between edges (HIGH→LOW→HIGH)
// and classify using calibrated thresholds for this ECM.
//
// Automatically skips single-byte glitches (208μs pulses).
func (s *AldlSerial) ReadAldlBitEdgeBased() (byte, error) {
	const maxRetries = 10

	for i := 0; i < maxRetries; i++ {
		pulseMicros, err := s.MeasureEdgeTiming()
		if err != nil {
			return 0, err
		}

		bit, err := s.DecodeAldlBitArduino(pulseMicros)
		if err != nil {
			// Likely a 208μs glitch, skip and try next pulse
			continue
		}

		return bit, nil
	}

	return 0, errors.NewTimeout("too many invalid pulses")
}

// ReadAldlBitRunLength reads and decodes a single ALDL bit using run-length pulse measurement
//
// DEPRECATED: Use ReadAldlBitEdgeBased() for better accuracy
//
// This approach counts consecutive LOW bytes (0x00) to measure pulse width across multiple
// UART bytes. At 4800 baud, each byte is ~208μs:
// - Short LOW pulse (bit 0): ~368μs ≈ 1.77 bytes of 0x00
// - Long LOW pulse (bit 1): ~4400μs ≈ 21 bytes of 0x00
func (s *AldlSerial) ReadAldlBitRunLength() (byte, error) {
	runLength, err := s.MeasurePulseWidthRunLength()
	if err != nil {
		return 0, err
	}

	return s.DecodeAldlBitRunLength(runLength)
}

// MeasurePulseWidthRunLength measures pulse width by counting consecutive LOW bytes (0x00)
//
// DEPRECATED: Use MeasureEdgeTiming() for better accuracy
//
// Returns the number of consecutive 0x00 bytes seen.
func (s *AldlSerial) MeasurePulseWidthRunLength() (int, error) {
	const highByte = 0xFE
	const lowByte = 0x00
	const maxSkip = 50

	for i := 0; i < maxSkip; i++ {
		b, err := s.ReadByte()
		if err != nil {
			return 0, err
		}
		if b == lowByte {
			runLength := 1
			for {
				b, err := s.ReadByte()
				if err != nil {
					return 0, err
				}
				if b == lowByte {
					runLength++
				} else {
					return runLength, nil
				}
				if runLength > 300 {
					return runLength, nil
				}
			}
		}
	}

	return 0, errors.NewTimeout("no pulse found")
}

// MeasureEdgeTiming mimics Arduino's interrupt-based edge timing approach
//
// Arduino measures time between edges with microsecond precision.
// We do the same by detecting edges in the UART byte stream and counting bytes.
//
// At 4800 baud: 1 byte = ~208μs, so we can calculate precise pulse durations.
// Returns pulse duration in microseconds.
func (s *AldlSerial) MeasureEdgeTiming() (int, error) {
	const highByte = 0xFE     // HIGH state in UART sampling
	const lowByte = 0x00      // LOW state in UART sampling (this is the PULSE)
	const microsPerByte = 208 // At 4800 baud: 1 second / 4800 bytes = 208.33 μs/byte
	const maxSkip = 100       // Max bytes to read looking for falling edge

	// Step 1: Find falling edge (HIGH → LOW transition)
	// Skip bytes until we see a transition from 0xFE to 0x00
	prevByte := byte(0xFF) // Invalid initial state

	for i := 0; i < maxSkip; i++ {
		b, err := s.ReadByte()
		if err != nil {
			return 0, err
		}

		// Detect falling edge
		if prevByte == highByte && b == lowByte {
			// Found falling edge! Now measure LOW pulse duration
			byteCount := 1 // We've already read the first LOW byte

			// Step 2: Count bytes until rising edge (LOW → HIGH transition)
			for {
				b, err := s.ReadByte()
				if err != nil {
					return 0, err
				}

				if b == lowByte {
					byteCount++
				} else if b == highByte {
					// Rising edge found! Calculate pulse duration
					pulseMicros := byteCount * microsPerByte
					return pulseMicros, nil
				}
				// Ignore other byte values (shouldn't happen with clean signal)

				// Safety limit
				if byteCount > 300 {
					pulseMicros := byteCount * microsPerByte
					return pulseMicros, nil
				}
			}
		}

		prevByte = b
	}

	return 0, errors.NewTimeout("no falling edge found")
}

// DecodeAldlBitArduino decodes an ALDL bit using edge-based timing
//
// Thresholds tuned for actual hardware (1227747 ECM via PL2303 on macOS):
// - Bit 0: 400-1200μs (observed: 416, 624, 832, 1040μs)
// - Bit 1: 1800-2000μs (observed: 1872μs - matches Arduino)
//
// Original Arduino thresholds (360-370μs for bit 0) don't match this ECM.
// ECMs can have different pulse widths - bot-thoughts article mentions
// "timings vary across ECMs and require calibration"
func (s *AldlSerial) DecodeAldlBitArduino(pulseMicros int) (byte, error) {
	const (
		bit0MinMicros = 300  // Allow some tolerance below observed minimum
		bit0MaxMicros = 1300 // Upper bound for short pulses
		bit1MinMicros = 1700 // Lower bound for long pulses
		bit1MaxMicros = 2100 // Allow some tolerance above observed maximum
	)

	if pulseMicros >= bit0MinMicros && pulseMicros <= bit0MaxMicros {
		return 0, nil // Short pulse = bit 0
	} else if pulseMicros >= bit1MinMicros && pulseMicros <= bit1MaxMicros {
		return 1, nil // Long pulse = bit 1
	}

	// Pulse is outside expected ranges
	if pulseMicros < bit0MinMicros {
		// Too short - likely single-byte glitch
		return 0, errors.NewInvalidFrame(fmt.Sprintf("pulse too short: %dμs", pulseMicros))
	} else if pulseMicros > bit1MaxMicros {
		// Very long pulse - could be multiple bit 1s or sync
		// Classify as bit 1
		return 1, nil
	}

	return 0, errors.NewInvalidFrame(fmt.Sprintf("ambiguous pulse: %dμs", pulseMicros))
}

// DecodeAldlBitRunLength decodes an ALDL bit from LOW pulse width (run-length of 0x00 bytes)
//
// DEPRECATED: Use DecodeAldlBitArduino() for better accuracy
//
// At 4800 baud, each byte ≈ 208μs:
// - ALDL bit 0: Short LOW pulse ~368μs ≈ 1.77 bytes (threshold: 1-5 bytes)
// - ALDL bit 1: Long LOW pulse ~4400μs ≈ 21 bytes (threshold: 15+ bytes)
func (s *AldlSerial) DecodeAldlBitRunLength(runLength int) (byte, error) {
	// Threshold: 1-7 bytes = bit 0, 8+ bytes = bit 1
	if runLength >= 1 && runLength <= 7 {
		return 0, nil
	}
	return 1, nil
}

// BitBuffer holds a stream of decoded ALDL bits for 9-bit byte processing
type BitBuffer struct {
	bits []byte
	pos  int
}

// NewBitBuffer creates a new bit buffer
func NewBitBuffer() *BitBuffer {
	return &BitBuffer{
		bits: make([]byte, 0, 1000),
		pos:  0,
	}
}

// AddBit adds a bit to the buffer
func (bb *BitBuffer) AddBit(bit byte) {
	bb.bits = append(bb.bits, bit)
}

// Len returns the number of bits in the buffer
func (bb *BitBuffer) Len() int {
	return len(bb.bits) - bb.pos
}

// GetBit gets a bit at the current position and advances
func (bb *BitBuffer) GetBit() (byte, error) {
	if bb.pos >= len(bb.bits) {
		return 0, errors.NewTimeout("bit buffer empty")
	}
	bit := bb.bits[bb.pos]
	bb.pos++
	return bit, nil
}

// PeekBits looks ahead at N bits without consuming them
func (bb *BitBuffer) PeekBits(n int) []byte {
	end := bb.pos + n
	if end > len(bb.bits) {
		end = len(bb.bits)
	}
	return bb.bits[bb.pos:end]
}

// Skip advances the position by n bits
func (bb *BitBuffer) Skip(n int) {
	bb.pos += n
	if bb.pos > len(bb.bits) {
		bb.pos = len(bb.bits)
	}
}

// FindSyncPattern searches for the 9-bit sync pattern (0x1FF = 9 consecutive 1s)
// Returns true and the position if found, false otherwise
func (bb *BitBuffer) FindSyncPattern() (bool, int) {
	for i := bb.pos; i <= len(bb.bits)-9; i++ {
		// Check if we have 9 consecutive 1s
		allOnes := true
		for j := 0; j < 9; j++ {
			if bb.bits[i+j] != 1 {
				allOnes = false
				break
			}
		}
		if allOnes {
			return true, i
		}
	}
	return false, -1
}

// Read9BitByte reads a 9-bit ALDL byte (1 mode bit + 8 data bits)
// Returns (modeBit, dataByte, error)
func (bb *BitBuffer) Read9BitByte() (byte, byte, error) {
	if bb.Len() < 9 {
		return 0, 0, errors.NewTimeout("not enough bits for 9-bit byte")
	}

	// First bit is the mode bit
	modeBit, err := bb.GetBit()
	if err != nil {
		return 0, 0, err
	}

	// Next 8 bits are the data byte (MSB first)
	var dataByte byte
	for i := 7; i >= 0; i-- {
		bit, err := bb.GetBit()
		if err != nil {
			return 0, 0, err
		}
		if bit == 1 {
			dataByte |= (1 << i)
		}
	}

	return modeBit, dataByte, nil
}

// AldlReader9Bit handles 9-bit ALDL protocol decoding
type AldlReader9Bit struct {
	serial    *AldlSerial
	bitBuffer *BitBuffer
	synced    bool
}

// NewAldlReader9Bit creates a new 9-bit ALDL reader
func NewAldlReader9Bit(serial *AldlSerial) *AldlReader9Bit {
	return &AldlReader9Bit{
		serial:    serial,
		bitBuffer: NewBitBuffer(),
		synced:    false,
	}
}

// FillBuffer reads bits from serial and adds to buffer
// Uses edge-based timing for Arduino-compatible decoding
func (r *AldlReader9Bit) FillBuffer(minBits int) error {
	for r.bitBuffer.Len() < minBits {
		bit, err := r.serial.ReadAldlBitEdgeBased()
		if err != nil {
			return err
		}
		r.bitBuffer.AddBit(bit)
	}
	return nil
}

// FindSync searches for the sync pattern and aligns to 9-bit boundaries
func (r *AldlReader9Bit) FindSync() error {
	const maxSearchBits = 1000

	// Fill buffer with enough bits to search
	if err := r.FillBuffer(maxSearchBits); err != nil {
		return err
	}

	// Search for sync pattern
	found, pos := r.bitBuffer.FindSyncPattern()
	if !found {
		return errors.NewTimeout("sync pattern not found in buffer")
	}

	// Skip to the position after the sync
	r.bitBuffer.Skip(pos + 9)
	r.synced = true

	return nil
}

// ReadFrame reads a 20-byte ALDL frame (assumes already synced)
func (r *AldlReader9Bit) ReadFrame() ([]byte, error) {
	if !r.synced {
		if err := r.FindSync(); err != nil {
			return nil, err
		}
	}

	frame := make([]byte, 20)

	for i := 0; i < 20; i++ {
		// Ensure we have enough bits
		if err := r.FillBuffer(9); err != nil {
			return nil, err
		}

		modeBit, dataByte, err := r.bitBuffer.Read9BitByte()
		if err != nil {
			return nil, err
		}

		// Data bytes should have mode bit = 0
		// If we see mode bit = 1, it's another sync character
		if modeBit == 1 {
			// Hit another sync, we're out of alignment
			// Re-sync and start over
			r.synced = false
			return nil, errors.NewInvalidFrame("unexpected sync character in frame")
		}

		frame[i] = dataByte
	}

	return frame, nil
}

// AvailablePorts returns a list of available USB serial ports
func AvailablePorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, errors.WrapSerialPort(err, "failed to list ports")
	}

	// Filter for USB serial devices (platform-specific patterns)
	var usbPorts []string
	for _, port := range ports {
		isUSB := false

		// Linux: /dev/ttyUSB* or /dev/ttyACM*
		if len(port) >= 11 && (port[:11] == "/dev/ttyUSB" || port[:11] == "/dev/ttyACM") {
			isUSB = true
		}

		// Windows: COM*
		if len(port) >= 3 && port[:3] == "COM" {
			isUSB = true
		}

		// macOS: Filter for known USB-to-serial chipsets only
		// Common chipsets: PL2303, FTDI, CH340, CP210x, usbserial
		if len(port) >= 8 && port[:8] == "/dev/cu." {
			name := port[8:]
			if len(name) >= 7 && name[:7] == "PL2303-" ||
				len(name) >= 9 && name[:9] == "usbserial" ||
				len(name) >= 4 && name[:4] == "SLAB" || // Silicon Labs CP210x
				len(name) >= 5 && name[:5] == "wchusbserial" || // CH340
				len(name) >= 11 && name[:11] == "usbmodem" {
				isUSB = true
			}
		}

		if isUSB {
			usbPorts = append(usbPorts, port)
		}
	}

	return usbPorts, nil
}

// AllPortsDebug returns detailed information about all serial ports (for debugging)
func AllPortsDebug() ([]string, error) {
	return serial.GetPortsList()
}
