# Running goaldl on Windows with Legacy PL2303 Driver

## Why Windows?

WinALDL works successfully on Windows with the legacy Prolific PL2303 driver (version 3.3.11.152). This driver has better timing characteristics than the macOS driver, which should resolve the pulse width measurement issues we've been experiencing.

## Prerequisites

1. **Legacy PL2303 Driver (Critical!)**
   - Download from: https://github.com/johnstevenson/pl2303-legacy
   - Version: **3.3.11.152** (same version WinALDL uses)
   - This older driver has better timing precision than newer versions

2. **Windows PC** with:
   - USB port for PL2303 adapter
   - PowerShell or Command Prompt

## Installation Steps

### 1. Install Legacy PL2303 Driver

```powershell
# Download and extract the legacy driver
# Install using Device Manager:
# 1. Right-click on "Prolific USB-to-Serial Comm Port" in Device Manager
# 2. Update Driver > Browse my computer > Let me pick
# 3. Select "Prolific USB-to-Serial Comm Port (COM#)" version 3.3.11.152
```

### 2. Copy Compiled Binaries to Windows

Transfer these files from your Mac to Windows:
- `goaldl.exe` - Main application
- `testedge.exe` - Edge-based decoder test
- `searchprom.exe` - PROM ID searcher
- `syncsearch.exe` - Sync pattern searcher

### 3. Identify COM Port

```powershell
# List all COM ports
goaldl.exe ports

# Example output:
# Available serial ports:
#   COM3
#   COM4
```

## Testing with Legacy Driver

### Step 1: Test Edge-Based Decoder

This will show if the legacy driver provides better pulse timing:

```powershell
testedge.exe

# Expected output with good driver:
# Pulse   0: 365μs -> bit 0  ✓ Bit 0 (exact match)
# Pulse   1: 1872μs -> bit 1  ✓ Bit 1 (exact match)
```

**Success indicators:**
- Bit 0 pulses should be **consistent 365-370μs** (not 416-1040μs like macOS)
- Bit 1 pulses should be **1850-1899μs**
- Few or no 208μs glitch pulses

### Step 2: Search for Sync Pattern

```powershell
syncsearch.exe

# If successful:
# ✓✓✓ FOUND SYNC! 9 consecutive long pulses
```

### Step 3: Search for PROM ID

```powershell
searchprom.exe

# If successful:
# ✓ FOUND at bit offset X!
# Bytes: [18] [93]
```

### Step 4: Full Scan Test

```powershell
goaldl.exe scan -p COM3

# Expected output:
# Found sync character at frame boundary
# PROM ID: 6291 (0x1893) - matches 1227747 ECM
# Coolant: 180°F
# RPM: 600
# ...
```

## Troubleshooting

### "Driver version is too new"

If you see timing issues:
1. Check driver version in Device Manager
2. Should be **3.3.11.152** exactly
3. Newer versions (3.4.x, 3.5.x) have known timing problems

### "Serial port busy"

```powershell
# Close WinALDL or other programs using the port
# Then retry
```

### Still seeing 208μs glitches

The legacy driver should eliminate most glitches. If you still see them:
- Check USB cable quality
- Try different USB port
- Verify ECM is powered and running (engine idling or ignition in RUN)

## Comparison: macOS vs Windows

| Metric | macOS (Current) | Windows (Expected with Legacy Driver) |
|--------|-----------------|--------------------------------------|
| Bit 0 pulse | 416-1040μs (inconsistent) | 365-370μs (tight) |
| Bit 1 pulse | 1872μs ✓ | 1850-1899μs ✓ |
| Glitches | ~50% of pulses | <5% of pulses |
| Sync detection | Never found | Should work |
| PROM ID | Not found | Should work |

## Next Steps After Testing

If Windows testing is successful:

1. **Confirm edge-based decoder works** with consistent pulse widths
2. **Find sync pattern** (9 consecutive bit-1 pulses)
3. **Decode PROM ID** [18] [93] correctly
4. **Read full frames** with all sensor data
5. **Start data logging** to CSV/JSON

## Building from Source on Windows

If you need to build from source on Windows:

```powershell
# Install Go from https://go.dev/dl/
# Clone the repository
git clone <repo-url>
cd goaldl

# Build all tools
go build -o goaldl.exe ./cmd/goaldl
go build -o testedge.exe ./cmd/testedge
go build -o searchprom.exe ./cmd/searchprom
go build -o syncsearch.exe ./cmd/syncsearch
```

## References

- Legacy PL2303 Driver: https://github.com/johnstevenson/pl2303-legacy
- ALDL Protocol: https://www.bot-thoughts.com/2018/01/decoding-gms-aldl-with-teensy-36.html
- WinALDL: Uses same driver version for successful communication
