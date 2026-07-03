package logging

import (
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"goaldl/pkg/ecm"
	"goaldl/pkg/errors"
)

// Format represents the log output format
type Format int

const (
	// FormatCSV outputs timestamp and sensor values in CSV format
	FormatCSV Format = iota
	// FormatJSON outputs full metadata in JSON Lines format
	FormatJSON
	// FormatRaw outputs detailed byte breakdown with parsed values
	FormatRaw
	// FormatHexOnly outputs raw hex bytes only (one frame per line)
	FormatHexOnly
)

// Logger handles data logging in multiple formats
type Logger struct {
	format Format
	file   *os.File
	csv    *csv.Writer
}

// LogEntry represents a JSON log entry
type LogEntry struct {
	Timestamp       string             `json:"timestamp"`
	ECM             string             `json:"ecm"`
	RawDataHex      string             `json:"raw_data_hex"`
	ParsedValues    map[string]float64 `json:"parsed_values"`
}

// New creates a new data logger with the specified format
func New(outputPath string, format Format) (*Logger, error) {
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, errors.WrapIO(err, "failed to create log file")
	}

	logger := &Logger{
		format: format,
		file:   file,
	}

	// For CSV format, initialize CSV writer and write header
	if format == FormatCSV {
		logger.csv = csv.NewWriter(file)
		header := []string{
			"timestamp",
			"coolant_temp",
			"manifold_pressure",
			"throttle_position",
			"engine_rpm",
			"vehicle_speed",
			"oxygen_sensor",
			"battery_voltage",
		}
		if err := logger.csv.Write(header); err != nil {
			file.Close()
			return nil, errors.WrapCSV(err, "failed to write CSV header")
		}
		logger.csv.Flush()
	}

	return logger, nil
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.csv != nil {
		l.csv.Flush()
	}
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// LogData writes ECM data to the log file in the configured format
func (l *Logger) LogData(data *ecm.Data) error {
	switch l.format {
	case FormatCSV:
		return l.writeCSV(data)
	case FormatJSON:
		return l.writeJSON(data)
	case FormatRaw:
		return l.writeRaw(data)
	case FormatHexOnly:
		return l.writeHexOnly(data)
	default:
		return errors.NewConfig(fmt.Sprintf("unknown log format: %d", l.format))
	}
}

// writeCSV writes data in CSV format
func (l *Logger) writeCSV(data *ecm.Data) error {
	record := []string{
		time.Now().Format(time.RFC3339),
		fmt.Sprintf("%.1f", data.ParsedValues["coolant_temp"]),
		fmt.Sprintf("%.4f", data.ParsedValues["map_voltage"]),
		fmt.Sprintf("%.4f", data.ParsedValues["tps_voltage"]),
		fmt.Sprintf("%.0f", data.ParsedValues["engine_rpm"]),
		fmt.Sprintf("%.0f", data.ParsedValues["vehicle_speed"]),
		fmt.Sprintf("%.2f", data.ParsedValues["oxygen_sensor"]),
		fmt.Sprintf("%.1f", data.ParsedValues["battery_voltage"]),
	}

	if err := l.csv.Write(record); err != nil {
		return errors.WrapCSV(err, "failed to write CSV record")
	}
	l.csv.Flush()
	return nil
}

// writeJSON writes data in JSON Lines format
func (l *Logger) writeJSON(data *ecm.Data) error {
	entry := LogEntry{
		Timestamp:    time.Now().Format(time.RFC3339),
		ECM:          data.EcmDefinition.PartNumber,
		RawDataHex:   hex.EncodeToString(data.RawData),
		ParsedValues: data.ParsedValues,
	}

	encoder := json.NewEncoder(l.file)
	if err := encoder.Encode(entry); err != nil {
		return errors.WrapJSON(err, "failed to write JSON entry")
	}
	return nil
}

// writeRaw writes detailed byte breakdown with parsed values
func (l *Logger) writeRaw(data *ecm.Data) error {
	timestamp := time.Now().Format(time.RFC3339)

	// Write timestamp
	if _, err := fmt.Fprintf(l.file, "=== Frame at %s ===\n", timestamp); err != nil {
		return errors.WrapIO(err, "failed to write raw entry")
	}

	// Write hex data
	if _, err := fmt.Fprintf(l.file, "Raw Hex: %s\n", hex.EncodeToString(data.RawData)); err != nil {
		return errors.WrapIO(err, "failed to write raw hex")
	}

	// Write parsed values
	if _, err := fmt.Fprintln(l.file, "Parsed Values:"); err != nil {
		return errors.WrapIO(err, "failed to write parsed header")
	}

	for _, param := range data.EcmDefinition.Parameters {
		value, ok := data.ParsedValues[param.Name]
		if ok {
			if _, err := fmt.Fprintf(l.file, "  %s: %.2f %s (%s)\n",
				param.Name, value, param.Unit, param.Description); err != nil {
				return errors.WrapIO(err, "failed to write parameter")
			}
		}
	}

	if _, err := fmt.Fprintln(l.file); err != nil {
		return errors.WrapIO(err, "failed to write newline")
	}

	return nil
}

// writeHexOnly writes raw hex bytes only (one frame per line)
func (l *Logger) writeHexOnly(data *ecm.Data) error {
	hexStr := hex.EncodeToString(data.RawData)
	if _, err := fmt.Fprintln(l.file, hexStr); err != nil {
		return errors.WrapIO(err, "failed to write hex data")
	}
	return nil
}

// ParseFormat parses a format string into a Format enum
func ParseFormat(formatStr string) (Format, error) {
	switch formatStr {
	case "csv":
		return FormatCSV, nil
	case "json":
		return FormatJSON, nil
	case "raw":
		return FormatRaw, nil
	case "hex":
		return FormatHexOnly, nil
	default:
		return 0, errors.NewConfig(fmt.Sprintf("unknown format: %s (must be csv, json, raw, or hex)", formatStr))
	}
}
