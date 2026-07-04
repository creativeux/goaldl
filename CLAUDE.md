# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

goaldl is a cross-platform Go implementation of an ALDL (Assembly Line Diagnostic Link) protocol scanner and datalogger for GM ECMs (160-baud pre-OBD2 data stream, primary target: GM 1227747). Based on the open-source linuxaldl and rustaldl projects, with a WinALDL log from the real vehicle as ground truth.

## Development Commands

- `go build ./...` / `go vet ./...` / `go test ./...` / `go fmt ./...`
- `go run ./cmd/goaldl` - Show available commands
- `go run ./cmd/goaldl ports` - List available USB serial ports (name drifts; check before using -p)
- `go run ./cmd/goaldl record -p /dev/cu.usbserial-10 -t 60 -o session.raw` - **Capture raw bytes to a file (do this first at the car)**
- `go run ./cmd/goaldl monitor pkg/decoder/testdata/drive_4800.raw` - Live sensor table (sensor / raw / value), replaying a capture
- `go run ./cmd/goaldl monitor -p /dev/cu.usbserial-10 -o session.raw -csv live.csv` - Same table live from the ECM; `-o` records raw, `-csv` logs decoded frames (this is the live datalogger — replaces the old `scan`/`log`)
- `go run ./cmd/goaldl decode session.raw -o frames.csv` - Batch-decode a capture file to frames + CSV (offline only; use `monitor -p` for live)
- `go run ./cmd/goaldl blm session.raw -o correction.csv` - Build the BLM fuel-trim table (rich/lean by RPM × load) from a capture
- `go run ./cmd/goaldl monitor -p /dev/cu.usbserial-10 -blm -o session.raw` - Watch the BLM grid fill live while driving (records raw too)
- `go run ./cmd/goaldl simulate -n 10` - Generate a synthetic capture for testing decode without hardware
- `go test ./pkg/decoder -run TestGolden -update` - Regenerate golden files after an intended decoder change (review the diff before committing)

The command surface is deliberately small: **record** (capture raw), **monitor** (live/replay table, +raw record, +CSV log), **decode** (offline batch decode+export), **simulate** (test data), **ports**/**ecms** (info). `monitor` owns everything live; `decode` is file-only.

## Architecture

The codebase was consolidated (2026-07-03) down to the working core; all experimental decoders and one-off diagnostic tools were deleted (recoverable via git history).

- `pkg/decoder/` - **The decoder** (byte-value state machine) + synthetic encoder + tests. Start here.
  - `testdata/` - real raw captures (`idle_4800.raw`, `drive_4800.raw`) as committed fixtures, plus their `.golden` frame dumps. These are the root of the test suite.
- `cmd/goaldl/` - CLI: `main.go` (ports/ecms) + `capture.go` (record/decode/simulate) + `monitor.go` (live table) + `csv.go` (shared `frameCSV` writer used by decode and monitor)
- `pkg/stream/` - Provider abstraction feeding a live sensor table: `ReplayProvider` (from a capture file) and `SerialProvider` (live ECM, with optional raw recording) drive the same `Renderer`. `BuildRows` (pure) and the providers are unit-tested against the drive fixture.
- `pkg/blm/` - BLM (Block Learn Multiplier / long-term fuel trim) accumulator: bins readings into an RPM × MAP grid and emits Samples/Average/Correction tables (matching `data/20250601_162123_BLM.txt`). The tuning metric is **Wide Average** — the mean BLM over every valid sample in a cell; target 128, >128 = lean, <128 = rich. Correction = avg/128 (multiply base VE/fuel by it). Frame→sample extraction + closed-loop/BLM-enable gating + MAP-volts→kPa live in `pkg/ecm/fueltrim.go` (`FuelTrimSample`, `MapVoltsToKPa`), shared by the `blm` command and the live view. **One assumption to verify vs WinALDL:** the MAP-volts→kPa transfer (`MapVoltsToKPa`); binning/correction don't depend on it. (The reference file's Narrow + Avg10/StdDev variants are intentionally not built — Wide Average is the metric used for tuning.)
- Live BLM view: `monitor -blm` streams frames through `pkg/stream`'s `BLMView`, which drives the same `pkg/blm.Grid`, redraws a compact heatmap in place on a TTY (active cell reverse-highlighted, closed/open-loop status, active-cell progress `n/min`), and prints the final Average + Correction tables on exit.
- **Confidence threshold** (`blm.DefaultMinSamples` = 4, `-min` flag on both `blm` and `monitor -blm`): BLM hunts, so a cell isn't trusted until it has enough readings (WinALDL uses ~3-4). Below the threshold a live cell renders dim (accumulating) and its correction is held at 1.000 (no change); at/above it renders solid and its correction is applied. `Grid.CorrectionAtLeast(min)` and `Grid.PopulatedCells(min)` implement this.
- `pkg/ecm/` - ECM definitions and frame parsing (GM 1227747 per A033.ads; byte order verified against the WinALDL log)
- `pkg/serial/serial.go` - thin serial-port wrapper (`go.bug.st/serial`): open/read/flush/list. No decoding.
- `pkg/aldl/aldl.go` - just the shared `Frame` type
- `pkg/errors/` - error types
- `data/20250601_111156_LOG.txt` - **Ground truth**: WinALDL log from the real ECM (frame layout + plausible sensor values)
- Root `*.raw` / `*.csv` are gitignored working files; canonical fixtures live in `pkg/decoder/testdata/`.

## ALDL 160-Baud Protocol — CORRECT MODEL

The ALDL line idles HIGH. Each 6250μs bit cell (160 bps) starts with a falling edge and a LOW pulse whose **duration** encodes the bit:

- Logic 0: short pulse (~365μs on the 1227747)
- Logic 1: long pulse (~1875μs on the 1227747; **ECM-family-specific**, spec range ~1500-4400μs — always classify with one coarse threshold near 1100μs, never tight ranges)
- Characters: 9 bits = 1 mode bit (0 for data) + 8 data bits **MSB first**
- Sync: 0x1FF = nine consecutive 1-bits, the only place that pattern occurs; separates 20-byte frames
- Frame: 20 data bytes + sync = 189 bits ≈ 1.18s per frame (matches WinALDL log cadence ~1.2s)

### How a PC UART reads this signal (the key insight)

The interface cable inverts the signal onto the UART RX pin. Each ALDL pulse triggers exactly one UART character: the falling edge is the start bit, and the number of consecutive LOW data bits (LSB first) measures the pulse width **using the adapter chip's own hardware clock**. At 4800 baud (208μs/UART bit):

- Logic 0 (~365μs) → byte `0xFE`
- Logic 1 (~1875μs) → byte `0x00`
- **One byte per ALDL bit. Idle time between pulses produces no bytes at all.**

Consequences that MUST guide any decoder work:

1. **Decode from byte VALUES only, never from host-side timing.** Byte values are fixed by the adapter's hardware UART; USB/driver buffering only delays delivery. Host timestamps are useless (16ms-class latency vs 365μs pulses).
2. The byte stream is NOT a uniform-rate waveform sample. "1 byte = 208μs" is wrong (that's one UART *bit*; a framed byte spans ~2083μs, and idle gaps vanish). Never reconstruct pulse durations from byte counts.
3. Sync appears as **9 consecutive `0x00` bytes** at 4800 baud.
4. At 2400 baud the same one-byte-per-bit rule holds with different values (logic 0 → `0xFF`, logic 1 → ~`0xF0`). Tech Edge recommends 2400 for C3-era ECMs like the 1227747; 4800 is confirmed working with WinALDL on this hardware. Record at both and compare.

See `pkg/decoder/decoder.go` for the full model and edge cases (very long pulses on other ECMs can span two UART characters).

## History — why past sessions failed (do not repeat)

Months of debugging failed because every decoder attempt (ones-counting per byte, run-length across bytes, "edge timing" via byteCount×208μs) treated the byte stream as a timing record. Re-analysis showed the "macOS PL2303 driver jitter" diagnosis was wrong — the captured data was clean the whole time:

- The observed "perfect 1872μs bit-1 pulses (9 bytes)" were 9 consecutive `0x00` bytes = **the 0x1FF sync pattern**, consumed as a single bit
- The "208μs driver glitches (~50%)" were legitimate isolated 1-bits, discarded as noise
- "Never found 9 consecutive 1s" — because the sync run was being merged into one "bit"

WinALDL works over the identical cable (even in a VM on the same Mac) because byte values are timing-independent. `DECODER_STATUS.md` and `HARDWARE_DECODING.md` predate this diagnosis — treat their conclusions as historical record, not guidance.

## GM 1227747 Frame Layout (A033.ads, verified against WinALDL log)

Byte 0: MW2 (mode word; 128/132 typical) · 1-2: PROM ID (24, 147 → 6291) · 3: IAC steps · 4: Coolant temp (lookup) · 5: MPH · 6: MAP (×0.0196 V) · 7: RPM (×25) · 8: TPS (×0.0196 V) · 9: Integrator · 10: O2 (×4.44 mV) · 11-13: MALFFLG1-3 · 14: MWAF1 · 15: Battery (×0.1 V) · 16: MCU2IO · 17: Knock count · 18: BLM · 19: Rich/lean counter

Target idle conditions for sanity checks: ~600 RPM (raw 24), ~180°F warm coolant, 0 MPH, TPS ≈ 0.5V, battery ~13.5-14.5V running.

## Hardware

- User's adapter: PL2303 USB-serial (vendor 0x067B, product 0x2303), currently enumerating as `/dev/cu.usbserial-10` on macOS Apple Silicon (older sessions saw `/dev/cu.PL2303-USBtoUART210` — the name depends on the driver in use; check `goaldl ports`). macOS has **no built-in PL2303 driver** — requires Prolific's "PL2303 Serial" App Store DriverKit app; pre-2012/counterfeit PL2303HXA chips are driver-blocked.
- Fallback adapter if a capture shows corrupt bytes: genuine FTDI FT232R (native Apple driver).
- Future onboard option: MCU bridge (bot-thoughts method — falling-edge interrupt, sample pin 2000μs later, robust across ECM pulse widths). Particle Photon 2 is capable (needs 3.3V level shifting); Raspberry Pi ≤4 with pigpio works; pigpio does not support Pi 5.

## Current Status / Next Steps

1. ✅ `pkg/decoder` byte-value decoder implemented, unit-tested against ground-truth frames (round trip at 2400/4800 baud, inverted polarity, pulse-width variation, noise recovery)
2. ✅ record / decode / simulate pipeline working end to end on synthetic captures
3. ✅ **VALIDATED ON REAL HARDWARE (2026-07-03, macOS + PL2303 at 4800 baud)**: idle capture 159.0 bytes/sec, 99.98% clean 0xFE/0x00, 47/47 PROM match; 14-min drive capture = 635/635 PROM match across full operating range (RPM 425-3700, TPS idle→WOT, closed-loop fuel trim, multiple BLM cells). Live `decode -p` streams frames in real time. macOS PL2303 path works — no new hardware needed.
4. ✅ **CONSOLIDATED (2026-07-03)**: repo under git; deleted 23 experimental cmd tools, 6 dead decoders, `pkg/autoscan`, legacy `pkg/aldl` sync path, and 13 legacy subcommands (7214 → ~1775 lines). Test suite rooted in the real captures: exact-stats regression + golden frame dumps in `pkg/decoder/testdata/`.
5. **Data policy**: decoder is a faithful transport — NO plausibility filtering, outlier rejection, or smoothing in the decode path. Emit every structurally-aligned frame warts-and-all (the drive capture's one 221°F coolant spike and 3 tail 0V-battery frames are intentionally preserved). Quality signals ride alongside as fields (e.g. PROM-ID match); data-quality decisions belong to downstream consumers/visualization where they're tunable.
6. Next: BLM-map / visualization / consumer layer (this is where sanity checks live, per #5). VSS/vehicle-speed reads 0 on this vehicle — either not wired to the ECM or was captured stationary; not a decoder issue (byte 5 is genuinely 0x00).
7. Optional phase 2 (onboard datalogging): MCU bridge per Hardware section below.
