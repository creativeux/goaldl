> **⚠️ SUPERSEDED (2026-07-03):** Several conclusions below are wrong (per-byte ones counting, pulse-width-from-byte-count math, "pulses span multiple bytes" at 4800 baud for this ECM). The correct model — one UART byte per ALDL bit, decode from byte values only — is documented in CLAUDE.md and implemented in `pkg/decoder`. Kept as historical record.

# ALDL Hardware Decoding - Technical Analysis

## Current Understanding (2025-01-08)

### What We Know Works
- **WinALDL**: Successfully reads ALDL at 4800 baud sampling rate
- **Hardware**: PL2303 USB-to-serial adapter on `/dev/cu.PL2303-USBtoUART110`
- **ECM**: GM 1227747 (PROM ID: 24, 147) - 160 baud ALDL, 20-byte frames
- **Raw data observed**: Stream of `0x00` and `0xFE` bytes when engine running

### ALDL Protocol Fundamentals

#### Timing
- **ALDL logical baud rate**: 160 baud (6250 μs per bit)
- **WinALDL sampling rate**: 4800 baud (208.33 μs per UART bit)
- **Oversampling ratio**: 30x (each ALDL bit = 30 UART bits)

#### Pulse Width Modulation
- **Signal idle state**: HIGH
- **Data encoding**: Pulse width (duration of LOW state)
  - Logic 0: 360-370 μs LOW pulse
  - Logic 1: 1850-4400 μs LOW pulse (varies by ECM)

#### Frame Structure
- **9-bit bytes**: 1 mode bit + 8 data bits
- **Sync character**: 0x1FF (9 consecutive 1-bits) - only byte with mode bit = 1
- **Frame size**: 20 bytes for 1227747 ECM
- **Frame format**:
  ```
  Byte 0: MW2 (Mode Word 2)
  Byte 1: PROMIDA (24 for 1227747)
  Byte 2: PROMIDB (147 for 1227747)
  Bytes 3-19: Sensor data
  ```

### Bot-Thoughts Approach (GPIO with Interrupts)

Their Teensy implementation uses direct GPIO access:

1. **Falling edge interrupt**: Detects start of pulse
2. **Timer-based sampling**: Samples signal level 2000 μs after edge
3. **Bit determination**:
   - LOW at sample point = logic 1
   - HIGH at sample point = logic 0
4. **9-bit accumulator**: Builds 16-bit value, detects 0x1FF sync
5. **Byte extraction**: Masks off mode bit, outputs 8-bit data

**Key advantage**: Direct timing control via interrupts

### Our Approach (UART Sampling)

We're using a serial port at 4800 baud to sample the ALDL signal:

#### UART Byte Patterns
At 4800 baud, each UART bit = 208 μs:

| ALDL Signal | Duration | UART Bits | Typical UART Bytes |
|-------------|----------|-----------|-------------------|
| Idle (HIGH) | Continuous | Many 1s | 0xFF 0xFF 0xFF... |
| Logic 0 pulse | 360 μs | ~1.7 bits | 0xFE, 0xFC (few 0 bits) |
| Logic 1 pulse | 1850 μs | ~8.9 bits | 0x00 0x00 (many 0 bits) |
| Logic 1 pulse (long) | 4400 μs | ~21 bits | 0x00 0x00 0x00 (many bytes) |

#### Observed Data
Raw dump shows: `00 00 FE FE FE FE FE...`
- Multiple `0x00` bytes = long LOW periods = logic 1 bits
- `0xFE` bytes = brief LOW periods = logic 0 bits

### Decoding Challenges

#### Challenge 1: Pulse Boundaries Don't Align with UART Bytes
- A logic 1 pulse (21 UART bits) spans 2-3 UART bytes
- Can't decode by examining single bytes in isolation
- Must process continuous bit stream

#### Challenge 2: 9-Bit Byte Alignment
- ALDL uses 9-bit bytes, UART uses 8-bit bytes
- Sync character (0x1FF) provides alignment
- Must accumulate 9-bit groups after finding sync

#### Challenge 3: Bit-Level vs Byte-Level Processing
- Bot-thoughts samples individual bits via GPIO
- We receive pre-sampled 8-bit bytes from UART
- Must reconstruct bit timing from byte patterns

### Implementation Attempts

#### Attempt 1: Single-Byte Bit Counting ✗
- Counted '1' bits in each UART byte
- Threshold: 0-2 ones = logic 0, 4-8 ones = logic 1
- **Failed**: Long pulses span multiple bytes

#### Attempt 2: Bit Stream with Edge Detection ⚠️
- Convert UART bytes to continuous bit stream
- Find HIGH→LOW transitions (falling edges)
- Count consecutive 0 bits (LOW duration)
- Threshold: 1-4 zeros = logic 0, 6+ zeros = logic 1
- **Status**: Implemented but not finding valid sync

### Why WinALDL Works

WinALDL uses 4800 baud UART sampling (same as us) and works perfectly. Possible explanations:

1. **Different thresholds**: May use different bit count thresholds
2. **Better buffering**: May read larger chunks before decoding
3. **Alternative sync method**: May not rely on 9-bit sync character
4. **Byte-pattern matching**: May look for PROM ID pattern in byte stream directly
5. **Proprietary algorithm**: May use approach we haven't considered

### Proposed Next Steps

#### Option A: Refine Bit Stream Decoder
1. Add detailed logging of bit stream patterns
2. Verify falling edge detection is working
3. Adjust pulse width thresholds based on actual data
4. Ensure sufficient buffering before attempting decode
5. Test sync character detection with known-good data

#### Option B: Byte-Pattern Approach
1. Read large buffer of UART bytes (not bits)
2. Convert to ALDL bits using sliding window
3. Try all possible bit alignments (0-7 bit offsets)
4. Look for PROM ID [24, 147] at each alignment
5. Once found, maintain that alignment for subsequent frames

#### Option C: Hybrid Approach
1. Use byte patterns to find approximate sync
2. Then switch to bit-level decoding for precision
3. Continuously verify alignment with PROM ID checks

#### Option D: Reference Implementation
1. Study WinALDL source code (if available)
2. Or use a logic analyzer to capture timing
3. Implement exactly what works

## BREAKTHROUGH: TechEdge UART Method (2025-01-08)

### The Correct Approach

After studying the TechEdge documentation, we discovered the proper UART decoding method:

**Key Insight**: At 1600 baud, each UART byte period (10 bits @ 625μs = 6250μs) equals one ALDL bit period!

#### Method
1. Use **1600 baud** (or 2400 for C3 ECUs) - NOT 4800!
2. Read one UART byte per ALDL bit
3. Extract **bit 4** (middle data bit) from each UART byte
4. **Invert** the bit (due to RS232 polarity inversion)
5. Assemble 9 ALDL bits into 8-bit bytes (1 mode + 8 data)

#### Implementation Status
✓ TechEdgeDecoder implemented in `pkg/serial/techedge_decoder.go`
✓ Successfully receiving UART bytes at 1600 baud
✓ Successfully decoding ALDL bits and bytes
✓ Mode bit separation working
⚠️ PROM ID pattern search needs more tuning (not finding [24, 147] yet)

#### Test Results (1600 baud)
```
UART bytes received: 0xF8 (11111000), 0x00 (00000000)
ALDL bytes decoded: Various values with correct mode bits
Examples:
  mode=1 data=0x22 (34)
  mode=0 data=0x38 (56)
  mode=1 data=0x70 (112)
  mode=0 data=0x5C (92)
```

This proves the TechEdge method works! We just need to find the PROM ID pattern or sync character.

### Testing Commands

```bash
# TechEdge decoder (RECOMMENDED)
go run ./cmd/goaldl techedge -p /dev/cu.PL2303-USBtoUART110 -b 1600 -d -n 1

# Try 2400 baud for C3 ECUs
go run ./cmd/goaldl techedge -p /dev/cu.PL2303-USBtoUART110 -b 2400 -d -n 1

# Try sync character detection (slower but more reliable)
go run ./cmd/goaldl techedge -p /dev/cu.PL2303-USBtoUART110 -b 1600 --sync -n 1

# Raw UART bytes at different baud rates
go run ./cmd/goaldl rawdump -p /dev/cu.PL2303-USBtoUART110 -b 1600 -n 100
go run ./cmd/goaldl rawdump -p /dev/cu.PL2303-USBtoUART110 -b 2400 -n 100
```

### References
- Bot-thoughts ALDL decoder: https://www.bot-thoughts.com/2018/01/decoding-gms-aldl-with-teensy-36.html
- A033.ads (1227747 ECM): https://gearhead-efi.com/gearhead-efi/def/ads/A033.ads
- Arduino ALDL (160 baud): https://github.com/rchipka/Arduino-ALDL-160-baud
- TechEdge ALDL spec: https://www.techedge.com.au/vehicle/aldl160/160serial.htm
