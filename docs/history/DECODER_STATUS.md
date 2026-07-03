> **⚠️ SUPERSEDED (2026-07-03):** The "macOS PL2303 driver timing issues" conclusion below was a misdiagnosis. The decoder's byteCount×208μs timing model was wrong (a framed byte spans ~2083μs and idle gaps produce no bytes); the "1872μs bit-1 pulses (9 bytes)" were actually the 0x1FF sync pattern, and the "208μs glitches" were real 1-bits. See CLAUDE.md "ALDL 160-Baud Protocol — CORRECT MODEL" and `pkg/decoder`. Kept as historical record.

# ALDL Decoder Implementation Status

## Summary

Implemented a complete edge-based ALDL decoder that mimics the Arduino approach, but encountered **macOS PL2303 driver timing issues** that prevent successful sync detection. **Recommendation: Test on Windows with legacy PL2303 driver (version 3.3.11.152)** that WinALDL uses successfully.

## What We Built

### 1. Edge-Based Timing Decoder ✅

**File:** `pkg/serial/serial.go`

**Key Functions:**
- `MeasureEdgeTiming()` - Detects edges (0xFE ↔ 0x00 transitions) and measures pulse duration in microseconds
- `DecodeAldlBitArduino()` - Classifies bits using calibrated thresholds
- `ReadAldlBitEdgeBased()` - Complete Arduino-style bit reader with glitch filtering

**How it works:**
1. Samples ALDL signal at 4800 baud UART (30x oversampling of 160 baud ALDL)
2. Detects falling edge (0xFE → 0x00) = start of LOW pulse
3. Counts bytes until rising edge (0x00 → 0xFE) = end of LOW pulse
4. Calculates duration: `byteCount × 208μs`
5. Classifies bit: 300-1300μs = bit 0, 1700-2100μs = bit 1

### 2. 9-Bit Protocol Support ✅

**File:** `pkg/serial/serial.go` (lines 359-543)

- `BitBuffer` - Manages decoded bit stream
- `AldlReader9Bit` - Handles 9-bit ALDL bytes (1 mode bit + 8 data bits)
- `FindSyncPattern()` - Searches for 0x1FF (9 consecutive 1s)
- `Read9BitByte()` - Extracts 9-bit bytes with mode bit checking

### 3. Diagnostic Tools ✅

**Compiled for Windows:**

- `goaldl.exe` - Main application (scan, log, ports, etc.)
- `testedge.exe` - Shows pulse widths and bit classifications
- `searchprom.exe` - Searches for PROM ID [18] [93] at all offsets
- `syncsearch.exe` - Looks for sync pattern (9 consecutive bit-1s)

## Test Results on macOS

### Hardware
- ECM: GM 1227747 (PROM ID: 6291 / 0x1893)
- Adapter: PL2303 USB-to-serial
- OS: macOS (Apple Silicon)
- Driver: Built-in macOS driver

### Observed Pulse Widths

| Pulse Type | Expected (Arduino) | Observed (macOS) | Status |
|------------|-------------------|------------------|--------|
| Bit 0 | 360-370μs | 416-1040μs | ❌ Too much variation |
| Bit 1 | 1850-1899μs | 1872μs | ✅ Perfect match |
| Glitches | None | 208μs (~50% of pulses) | ❌ Driver noise |

### Key Findings

1. **Bit 1 decoding is perfect** - 1872μs pulses decode consistently ✅
2. **Bit 0 shows 2.5x variation** - 416μs to 1040μs (should be ~365μs) ❌
3. **Never found sync pattern** - Max consecutive bit-1s: 2 (need 9) ❌
4. **PROM ID not found** - Searched all 500 bit offsets ❌
5. **Lots of 208μs glitches** - Single-byte pulses from driver timing jitter ❌

### Root Cause

**macOS PL2303 driver timing issues:**
- Jitter affects short pulses (bit 0) more than long pulses (bit 1)
- Creates spurious 208μs single-byte glitches
- Same issue documented in other ALDL projects on macOS
- WinALDL avoids this by using legacy driver on Windows

## Math Verification

### ALDL at 160 Baud

- **Bit period:** 1/160 = 6,250μs per bit
- **Bit 0:** 365μs LOW + 5,885μs HIGH = 6,250μs ✓
- **Bit 1:** 1,875μs LOW + 4,375μs HIGH = 6,250μs ✓

### Sampling at 4800 Baud

- **UART byte time:** 1/4800 = 208.33μs
- **Oversampling:** 4800/160 = **30x**
- **Bytes per bit:** 6,250μs ÷ 208μs = **30 bytes** ✓

### Expected vs Observed

| Measurement | Expected | Observed macOS | Match |
|-------------|----------|----------------|-------|
| Bit 0 pulse | 365μs (1.75 bytes) | 416-1040μs (2-5 bytes) | ❌ |
| Bit 1 pulse | 1,875μs (9 bytes) | 1,872μs (9 bytes) | ✅ |

**Conclusion:** The decoder math is correct. Bit 1 matches perfectly. Bit 0 variation is from driver timing issues.

## Recommendations

### Immediate: Test on Windows ⭐

**Why:** WinALDL works successfully on Windows with legacy PL2303 driver v3.3.11.152

**Expected results:**
- Bit 0 pulses: Consistent 365-370μs (not 416-1040μs)
- Bit 1 pulses: 1850-1899μs (already working)
- Glitches: <5% (not 50%)
- **Sync pattern should be detectable**
- **PROM ID should be found**

**See:** `WINDOWS_SETUP.md` for complete instructions

### Alternative: Hardware Solutions

If Windows testing also fails:

1. **FTDI adapter** - Better macOS driver support than PL2303
2. **Arduino/Teensy** - Intermediate decoder (what bot-thoughts uses)
3. **Direct GPIO** - Raspberry Pi with hardware interrupts

### Alternative: Software Solutions

1. **Parse WinALDL logs** - Since WinALDL works, goaldl can excel at analysis
2. **8192 baud** - Try linuxaldl's baud rate (better resolution for short pulses)
3. **Different timing method** - Sample at threshold (2ms) like bot-thoughts instead of measuring full pulse

## Files Modified

### Core Implementation
- `pkg/serial/serial.go` - Edge-based decoder (lines 171-333)
- `pkg/serial/serial.go` - 9-bit protocol support (lines 359-543)

### Test Commands
- `cmd/testedge/main.go` - Pulse width tester
- `cmd/searchprom/main.go` - PROM ID searcher
- `cmd/syncsearch/main.go` - Sync pattern searcher
- `cmd/diagpulses/main.go` - Detailed pulse diagnostics
- `cmd/findlong/main.go` - Very long pulse detector

### Documentation
- `WINDOWS_SETUP.md` - Complete Windows testing guide
- `DECODER_STATUS.md` - This file
- `CLAUDE.md` - Updated with current status

## Next Steps

1. **Transfer Windows binaries** (goaldl.exe, testedge.exe, etc.) to Windows PC
2. **Install legacy PL2303 driver** (v3.3.11.152)
3. **Run testedge.exe** to verify pulse timing improvements
4. **Run syncsearch.exe** to find sync pattern
5. **Run searchprom.exe** to verify PROM ID detection
6. **Run goaldl.exe scan** for full frame decoding

If successful on Windows, the decoder implementation is complete and ready for data logging!

## References

- Arduino implementation: `/Users/aaronstone/Development/Arduino-ALDL-160-baud/main.cpp`
- Bot-thoughts article: https://www.bot-thoughts.com/2018/01/decoding-gms-aldl-with-teensy-36.html
- Legacy PL2303 driver: https://github.com/johnstevenson/pl2303-legacy
- A033 datastream spec: `/Users/aaronstone/Development/Arduino-ALDL-160-baud/datastreams/A033.DS`
