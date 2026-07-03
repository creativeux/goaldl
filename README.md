# goaldl

Go port of rustaldl - ALDL protocol scanner and datalogger for GM ECMs.

## About

This is a Go port of the [rustaldl](../rustaldl) codebase. It provides cross-platform ALDL (Assembly Line Diagnostic Link) protocol communication for GM ECMs, specifically targeting the GM 1227747 ECM (A033 TBI, 86-88 4.3/5.0/5.7L engines).

**⚠️ IMPORTANT NOTE:** The original Rust codebase reads data from the serial port successfully, but produces gibberish/inaccurate values. This Go port preserves the same implementation logic with extensive inline documentation highlighting potential problem areas for debugging. See the "Debugging Notes" section below.

## Features

- Real-time ALDL data scanning (160 baud PWM protocol)
- Multi-format data logging (CSV, JSON, Raw, HexOnly)
- Support for 12 sensors including:
  - Coolant temperature, Engine RPM, Vehicle speed
  - MAP/TPS voltages, O2 sensor, Battery voltage
  - IAC position, Integrator, BLM, Rich/Lean counter
- Test mode for offline hex file analysis
- Cross-platform serial port support (macOS, Linux, Windows)

## Project Structure

```
goaldl/
├── cmd/goaldl/main.go          # CLI application
├── pkg/
│   ├── errors/errors.go        # Error types
│   ├── serial/serial.go        # Serial port wrapper
│   ├── aldl/aldl.go           # ALDL protocol
│   ├── ecm/ecm.go             # ECM definitions
│   └── logging/logging.go      # Data loggers
├── data/                       # Test data files
└── go.mod                      # Go module definition
```

## Installation

```bash
# Install dependencies
go mod download

# Build
go build ./cmd/goaldl

# Or run directly
go run ./cmd/goaldl <command>
```

## Usage

### List available serial ports
```bash
goaldl ports
```

### Real-time scanning (10 frames)
```bash
goaldl scan -p /dev/ttyUSB0
```

### Continuous logging
```bash
# CSV format
goaldl log -p /dev/ttyUSB0 -o data.csv -f csv

# JSON format
goaldl log -p /dev/ttyUSB0 -o data.json -f json -c 1000

# Raw format with detailed output
goaldl log -p /dev/ttyUSB0 -o data.log -f raw
```

### Test with hex files
```bash
goaldl test data/varied_sensors.hex
```

### Convert hex to CSV
```bash
goaldl convert data/varied_sensors.hex -o output.csv -i 100
```

### List supported ECMs
```bash
goaldl ecms
```

## ALDL Protocol Details

- **Baud Rate:** 160 baud (logical) using PWM encoding
- **Sampling Rate:** 2400 baud UART (15x oversampling)
- **Bit Encoding:** Pulse Width Modulation
  - Logic 0: 360-370μs pulse → 0-2 '1' bits in UART byte
  - Logic 1: 1850-1899μs pulse → 4-8 '1' bits in UART byte
- **Frame Structure:** 9-bit characters, MSB first
- **Frame Size:** 20 bytes (standard) or 25 bytes (extended with BLM)

## Debugging Notes

### ⚠️ Known Issue: Gibberish Data

The original Rust implementation reads data from the serial port but produces inaccurate/gibberish values. This Go port maintains the same logic with detailed comments marking potential problem areas.

### Primary Suspects for Debugging

The codebase includes extensive `⚠️` comments marking potential issues. Key areas to investigate:

#### 1. Bit Decoding Logic (`pkg/serial/serial.go`)

**Location:** `DecodeAldlBit()` function

**Current Logic:**
- 0-2 ones → ALDL bit 0
- 3 ones → Error (ambiguous)
- 4-8 ones → ALDL bit 1

**Potential Issues:**
- Threshold may be incorrect for actual signal characteristics
- The "3 ones = error" case may be too strict (could be valid data)
- Serial timing may not align with ALDL bit boundaries
- Baud rate relationship (2400 vs 160) may need adjustment

**Debug Suggestions:**
```go
// Add logging to see actual one-counts:
fmt.Printf("DEBUG: ones=%d, decoded_bit=%d\n", ones, bit)

// Try different thresholds:
// - Maybe 3 ones should map to 0 or 1 instead of error
// - Maybe threshold should be at 5 instead of 4
```

#### 2. Sync Detection (`pkg/aldl/aldl.go`)

**Location:** `WaitForSync()` function

**Current Logic:**
- Pattern-based: looks for PROM ID [24, 147]
- Validates against expected sensor ranges

**Potential Issues:**
- May lock onto wrong data that appears valid
- PROM ID may differ for different ECM variations
- May sync to middle of frame instead of start
- Force-resync strategy may cause drift

**Debug Suggestions:**
```go
// Log what values we're actually seeing:
fmt.Printf("DEBUG: Checking sync, bytes 0-1: [%d, %d]\n", frame[0], frame[1])

// Try syncing on different patterns
// Or disable pattern matching and sync on timing
```

#### 3. Bit Ordering (`pkg/aldl/aldl.go`)

**Location:** `readAldlByte()` function

**Current Logic:**
- Reads bits MSB-first (bit 7 → bit 0)
- Shifts each bit into position

**Potential Issues:**
- Bit ordering might actually be LSB-first
- Bit shifting direction could be reversed

**Debug Suggestions:**
```go
// Try LSB-first instead:
for i := 0; i < 8; i++ {  // 0 to 7 instead of 7 to 0
    bit, err := p.serial.ReadAldlBit()
    if bit == 1 {
        result |= (1 << i)
    }
}
```

#### 4. Continuous Resync (`pkg/aldl/aldl.go`)

**Location:** `ContinuousRead()` function

**Current Logic:**
- Forces resync before reading each frame

**Potential Issues:**
- May cause unnecessary delays
- May lose alignment if sync pattern appears in data
- May not be necessary if protocol is well-defined

**Debug Suggestions:**
```go
// Try reading frames without resyncing:
// - Sync once at start
// - Read frames continuously without calling WaitForSync()
```

### Debugging Workflow

1. **Enable verbose logging** in the suspect functions
2. **Capture raw UART bytes** before decoding
3. **Compare with known-good captures** if available
4. **Test with logic analyzer** to verify actual PWM timing
5. **Try alternative thresholds/orderings** systematically

### Test Data

The `data/` directory contains real ALDL captures for testing:
- `varied_sensors.hex` - 252 frames of real data
- `A033.ads` - ECM definition file with sensor formulas
- Test files with BLM data

Use these to test changes without needing the physical hardware:
```bash
goaldl test data/varied_sensors.hex
```

## Differences from Rust Version

This Go port maintains the core logic and structure but differs in:

1. **Dependencies:** Uses `go.bug.st/serial` instead of `serialport` crate
2. **Error Handling:** Go-style error handling instead of `Result<T, E>`
3. **Concurrency:** Uses channels for `ContinuousRead()` instead of iterators
4. **CLI:** Simplified flag parsing with standard library instead of `clap`

## Supported ECMs

Currently supports:
- **GM 1227747** - A033 TBI ECM (86-88 4.3/5.0/5.7L)

## Hardware Requirements

- USB-to-ALDL cable/adapter
- Compatible GM vehicle with ALDL port (typically under dashboard)
- The adapter must support 160 baud ALDL communication

## License

GPL-3.0 (maintains compatibility with original rustaldl)

## References

- Original Rust implementation: `../rustaldl`
- A033.ads ECM definition: `data/A033.ads`
- ALDL protocol documentation in rustaldl README
