package autoscan

import (
	"fmt"
	"time"
	"goaldl/pkg/aldl"
	"goaldl/pkg/serial"
)

// Config represents a configuration to test
type Config struct {
	BaudRate       int
	ThresholdLow   int // Max ones count for bit 0
	ThresholdHigh  int // Min ones count for bit 1
	Description    string
}

// Result represents the result of testing a configuration
type Result struct {
	Config Config
	Score  int
	Frames []*aldl.Frame
	PromID [2]byte
	Error  error
}

// FrameScorer evaluates the quality of ALDL frames
type FrameScorer struct{}

// ScoreFrame evaluates a single frame for plausibility
func (fs *FrameScorer) ScoreFrame(data []byte) int {
	if len(data) < 20 {
		return 0
	}

	score := 0

	// Check for expected PROM ID [24, 147] at bytes 1-2 (byte 0 is MW2)
	if data[1] == 24 && data[2] == 147 {
		score += 100 // Strong match
	}

	// Check for plausible coolant temp (byte 4)
	// Valid range roughly 0-255 raw, but typical values 130-220 (40°F to 250°F)
	if data[4] >= 100 && data[4] <= 230 {
		score += 10
	}

	// Check for plausible vehicle speed (byte 5)
	// Should be 0-150 MPH, idle should be 0
	if data[5] <= 150 {
		score += 10
	}

	// Check for plausible RPM (byte 7)
	// Factor is 25, so idle at 600 RPM = 24, normal range 0-200 (0-5000 RPM)
	if data[7] >= 10 && data[7] <= 200 {
		score += 10
	}

	// Check for plausible MAP voltage (byte 6)
	// Factor is 0.0196, range 0-255 (0-5V), typical 50-255
	if data[6] >= 30 && data[6] <= 255 {
		score += 5
	}

	// Check for plausible TPS voltage (byte 8)
	// Factor is 0.0196, idle should be low (0-20)
	if data[8] <= 50 {
		score += 5
	}

	// Check for plausible O2 sensor (byte 10)
	// Factor is 4.44, raw 0-255 (0-1133 mV)
	if data[10] <= 255 {
		score += 5
	}

	// Check for plausible battery voltage (byte 15)
	// Factor is 0.1, typical 110-150 (11-15V)
	if data[15] >= 100 && data[15] <= 160 {
		score += 10
	}

	// Penalize too many 0xFF bytes (likely noise)
	ffCount := 0
	for _, b := range data {
		if b == 0xFF {
			ffCount++
		}
	}
	if ffCount > 10 {
		score -= 20
	}

	// Penalize too many 0x00 bytes (likely silence)
	zeroCount := 0
	for _, b := range data {
		if b == 0x00 {
			zeroCount++
		}
	}
	if zeroCount > 10 {
		score -= 20
	}

	return score
}

// ScoreFrames evaluates multiple frames for consistency
func (fs *FrameScorer) ScoreFrames(frames []*aldl.Frame) int {
	if len(frames) == 0 {
		return 0
	}

	totalScore := 0

	// Score each individual frame
	for _, frame := range frames {
		totalScore += fs.ScoreFrame(frame.Data)
	}

	// Bonus for PROM ID consistency (bytes 1-2, byte 0 is MW2)
	if len(frames) >= 2 {
		firstPromID := [2]byte{frames[0].Data[1], frames[0].Data[2]}
		allMatch := true
		for _, frame := range frames[1:] {
			if frame.Data[1] != firstPromID[0] || frame.Data[2] != firstPromID[1] {
				allMatch = false
				break
			}
		}
		if allMatch {
			totalScore += 50 // Consistency bonus
		}
	}

	return totalScore
}

// DefaultConfigs returns a list of configurations to test
func DefaultConfigs() []Config {
	return []Config{
		// 4800 baud variants (30x oversampling) - WinALDL uses this!
		{4800, 1, 4, "4800 baud, tight (0-1=0, 4-8=1)"},
		{4800, 2, 5, "4800 baud, standard (0-2=0, 5-8=1)"},
		{4800, 3, 6, "4800 baud, relaxed (0-3=0, 6-8=1)"},
		{4800, 1, 7, "4800 baud, strict (0-1=0, 7-8=1)"},
		{4800, 2, 4, "4800 baud, narrow (0-2=0, 4-8=1)"},
		{4800, 1, 5, "4800 baud, medium (0-1=0, 5-8=1)"},
		{4800, 2, 6, "4800 baud, wide (0-2=0, 6-8=1)"},
		{4800, 1, 6, "4800 baud, balanced (0-1=0, 6-8=1)"},

		// 2400 baud variants (15x oversampling)
		{2400, 2, 4, "2400 baud, conservative (0-2=0, 4-8=1)"},
		{2400, 3, 5, "2400 baud, relaxed (0-3=0, 5-8=1)"},
		{2400, 1, 7, "2400 baud, strict (0-1=0, 7-8=1)"},
		{2400, 3, 4, "2400 baud, tight middle (0-3=0, 4-8=1)"},

		// 1200 baud (7.5x oversampling)
		{1200, 1, 4, "1200 baud, standard (0-1=0, 4-8=1)"},
		{1200, 2, 5, "1200 baud, relaxed (0-2=0, 5-8=1)"},

		// 9600 baud (60x oversampling)
		{9600, 2, 8, "9600 baud, standard (0-2=0, 8+=1)"},
		{9600, 1, 6, "9600 baud, tight (0-1=0, 6+=1)"},

		// 8192 baud (like linuxaldl, ~51x oversampling)
		{8192, 2, 6, "8192 baud, standard (0-2=0, 6+=1)"},
		{8192, 1, 5, "8192 baud, tight (0-1=0, 5+=1)"},
	}
}

// TestConfig tests a specific configuration
func TestConfig(portName string, config Config, frameCount int) (*Result, error) {
	result := &Result{
		Config: config,
		Frames: make([]*aldl.Frame, 0, frameCount),
	}

	// Open serial port with configured baud rate
	ser, err := serial.NewWithBaudRate(portName, config.BaudRate)
	if err != nil {
		result.Error = err
		return result, err
	}
	defer ser.Close()

	// Create custom decoder with configured thresholds
	decoder := &CustomDecoder{
		serial:        ser,
		thresholdLow:  config.ThresholdLow,
		thresholdHigh: config.ThresholdHigh,
	}

	// Try to read frames
	for i := 0; i < frameCount; i++ {
		frame, err := decoder.ReadFrame()
		if err != nil {
			continue // Skip errors
		}
		result.Frames = append(result.Frames, frame)
	}

	// Score the frames
	scorer := &FrameScorer{}
	result.Score = scorer.ScoreFrames(result.Frames)

	// Extract PROM ID from first frame if available (bytes 1-2, byte 0 is MW2)
	if len(result.Frames) > 0 && len(result.Frames[0].Data) >= 3 {
		result.PromID = [2]byte{result.Frames[0].Data[1], result.Frames[0].Data[2]}
	}

	return result, nil
}

// CustomDecoder decodes ALDL data with custom thresholds
type CustomDecoder struct {
	serial        *serial.AldlSerial
	thresholdLow  int
	thresholdHigh int
}

// ReadFrame reads a single 20-byte ALDL frame
func (d *CustomDecoder) ReadFrame() (*aldl.Frame, error) {
	data := make([]byte, 20)

	for i := 0; i < 20; i++ {
		b, err := d.readByte()
		if err != nil {
			return nil, err
		}
		data[i] = b
	}

	return &aldl.Frame{
		Data: data,
		Timestamp: time.Now(),
	}, nil
}

// readByte reads one ALDL byte (8 bits, MSB first)
func (d *CustomDecoder) readByte() (byte, error) {
	var result byte

	// Read 8 bits, MSB first
	for i := 7; i >= 0; i-- {
		bit, err := d.readBit()
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

// readBit reads a single ALDL bit using configured thresholds
func (d *CustomDecoder) readBit() (byte, error) {
	ones, err := d.serial.MeasurePulseWidth()
	if err != nil {
		return 0, err
	}

	// Apply configured thresholds
	if ones <= d.thresholdLow {
		return 0, nil
	} else if ones >= d.thresholdHigh {
		return 1, nil
	} else {
		// Ambiguous - default to 0 to avoid errors
		return 0, nil
	}
}

// ScanAll tests all configurations and returns results sorted by score
func ScanAll(portName string, framesPerTest int, progressCallback func(int, int, Config)) ([]*Result, error) {
	configs := DefaultConfigs()
	results := make([]*Result, 0, len(configs))

	for i, config := range configs {
		if progressCallback != nil {
			progressCallback(i+1, len(configs), config)
		}

		result, err := TestConfig(portName, config, framesPerTest)
		if err != nil {
			// Still add failed results
			result = &Result{
				Config: config,
				Error:  err,
				Score:  -1,
			}
		}
		results = append(results, result)
	}

	// Sort by score (descending)
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// FormatResult formats a result for display
func FormatResult(result *Result) string {
	if result.Error != nil {
		return fmt.Sprintf("❌ %s - ERROR: %v", result.Config.Description, result.Error)
	}

	if result.Score <= 0 {
		return fmt.Sprintf("❌ %s - Score: %d (no valid frames)", result.Config.Description, result.Score)
	}

	return fmt.Sprintf("✓ %s - Score: %d, Frames: %d, PROM ID: [%d, %d]",
		result.Config.Description, result.Score, len(result.Frames),
		result.PromID[0], result.PromID[1])
}
