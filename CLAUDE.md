# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

goaldl is a cross-platform Go implementation of an ALDL (Assembly Line Diagnostic Link) protocol scanner and datalogger for GM ECMs (160-baud pre-OBD2 data stream, primary target: GM 1227747). Based on the open-source linuxaldl and rustaldl projects, with a WinALDL log from the real vehicle as ground truth.

## Development Commands

- `go build ./...` / `go vet ./...` / `go test ./...` / `go fmt ./...`
- **The bare `goaldl` command launches the interactive TUI dashboard** — it's the default face. A known command word (record/decode/monitor/blm/simulate/ports/ecms/version/help) as the first arg runs that command directly; anything else (a `-p` flag, a capture-file path, or nothing) is the dashboard.
- `go run ./cmd/goaldl -p /dev/cu.usbserial-10` - **Dashboard**, live from the ECM (tabs: sensors / BLM grid / flags / error codes / raw history; `-tps0`/`-tps100` calibrate the TPS % column)
- `go run ./cmd/goaldl pkg/decoder/testdata/drive_4800.raw` - Dashboard, replaying a capture (`-speed N` to scrub)
- `go run ./cmd/goaldl` (no args) - Dashboard; auto-connects if exactly one USB serial port is present
- `go run ./cmd/goaldl help` - Usage
- `go run ./cmd/goaldl ports` - List available USB serial ports (name drifts; check before using -p)
- `go run ./cmd/goaldl record -p /dev/cu.usbserial-10 -t 60 -o session.raw` - **Capture raw bytes to a file (do this first at the car)**
- `go run ./cmd/goaldl monitor -p /dev/cu.usbserial-10 -o session.raw -csv live.csv` - Streaming (non-interactive) sensor table; `-o` records raw, `-csv` logs decoded frames
- `go run ./cmd/goaldl decode session.raw -o frames.csv` - Batch-decode a capture file to frames + CSV
- `go run ./cmd/goaldl blm session.raw -o correction.csv` - Build the BLM fuel-trim table (rich/lean by RPM × load) from a capture
- `go run ./cmd/goaldl monitor -p /dev/cu.usbserial-10 -blm -o session.raw` - Streaming BLM grid while driving (records raw too)
- `go run ./cmd/goaldl simulate -n 10` - Generate a synthetic capture for testing decode without hardware
- `go test ./pkg/decoder -run TestGolden -update` - Regenerate golden files after an intended decoder change (review the diff before committing)

Command model: **bare `goaldl`** (or with a `-p`/file source) = the TUI dashboard, the primary UX. The **scripting commands** — record (capture raw), monitor (streaming table, +raw record, +CSV log), decode (offline batch decode+export), blm (fuel-trim table), simulate (test data), ports/ecms (info) — are top-level: `goaldl blm session.raw`. `main.go` dispatches on the first-arg command word, falling through to the dashboard otherwise.

## Releases & versioning

Releases are automated from the commit history — **do not tag, bump versions, or edit `CHANGELOG.md` by hand.** Full standard: `product-knowledge/standards/release/versioning.md`.

- **Commits on `main` MUST be [Conventional Commits](https://www.conventionalcommits.org)** (`feat:` / `fix:` / `feat!:` or a `BREAKING CHANGE:` footer; `chore:`/`docs:`/`refactor:`/`test:`/`ci:`/`build:` don't cut a release on their own). The type drives the semver bump. Pre-1.0 the config bumps a *patch* for `feat:` and only bumps the *minor* for a breaking change (never auto-`1.0.0`).
- **release-please** (`.github/workflows/release.yml`, `release-please-config.json`, `.release-please-manifest.json`) keeps an open "release PR" that bumps the version + updates `CHANGELOG.md`; merging it tags `vX.Y.Z` and cuts a GitHub Release.
- **GoReleaser** (`.goreleaser.yaml`) then builds macOS/Linux/Windows × amd64/arm64 binaries with the version injected via `-ldflags -X main.*` and appends them to that release. Dry-run: `goreleaser release --snapshot --clean`.
- **Version embedding**: `cmd/goaldl/version.go` (`version`/`commit`/`date`) is set by ldflags at release time and falls back to `runtime/debug.ReadBuildInfo()` for a plain `go build`. `goaldl version` / `--version` prints it — every binary self-identifies.

## Architecture

The codebase was consolidated (2026-07-03) down to the working core; all experimental decoders and one-off diagnostic tools were deleted (recoverable via git history).

- `pkg/decoder/` - **The decoder** (byte-value state machine) + synthetic encoder + tests. Start here.
  - `testdata/` - real raw captures (`idle_4800.raw`, `drive_4800.raw`) as committed fixtures, plus their `.golden` frame dumps. These are the root of the test suite.
- `cmd/goaldl/` - The `goaldl` binary. `main.go` dispatches on the first-arg command word (record/decode/monitor/blm/simulate/ports/ecms/version/help) to that `cmdX`; anything else falls through to `cmdTUI` (the dashboard, default). `tui.go` (dashboard), `capture.go` (record/decode/simulate), `monitor.go` (streaming table), `blm.go`, `csv.go` (shared writer). All `cmdX` take `args []string` so `main` can route them.
- **Core API / layering (the reusable engine multiple front-ends drive):** `pkg/stream`'s **`Session`** is the facade — `NewSession(provider, registry, ecmPart, promID)` then `Run(ctx, func(Snapshot))`. It composes provider → decode → parse into a stream of **`Snapshot`** (frame + parsed `Sensors` + `FuelTrim` + `PROMOK`), all plain serializable data with no UI. The TUI is one consumer; a future `serve` adapter (HTTP/WebSocket → web/mobile) would consume the same `Snapshot` stream. Terminal rendering (`SensorTable`, `BLMBody`, `Renderer`, `BLMView`) is presentation layered on top, not part of the core data path.
- `pkg/stream/` - `Session`/`Snapshot` (core API) + Provider abstraction: `ReplayProvider` (capture file) and `SerialProvider` (live ECM, optional raw recording) emit `FrameEvent`s. Pure content builders `SensorTable`/`BLMBody`/`FlagsBody`/`CodesBody`/`RawHistory` produce terminal-view strings shared by the `monitor` renderers and the `tui`. `Snapshot` carries `Sensors` + `FuelTrim` + `Flags` + `Codes` + PROM/parse quality flags — all plain serializable data. Providers, `Session`, `BuildRows`, the view builders, and pacing are unit-tested against the drive fixture and the ground-truth log.
- `cmd/goaldl/tui.go` - **Dashboard** (Bubble Tea), the default UX: eight tabs (keys 1-8) — sensor table (dual-unit ALT column + always-on MIN/MAX), BLM / INT / O2 fuel-trim grids, spark-counts grid (knock deltas on WinALDL's finer spark axes RPM 400-3600/400 × MAP 30-100/5), flag data (MW2/MWAF1/MCU2IO checklists), error codes (MALFFLG1-3), and a WinALDL-style scrolling raw-byte history — driven by a `stream.Session` (live `-p` or replay). A **persistent loop-state line** (green CLOSED / amber OPEN + per-grid ●/○ recording dots incl. SPARK) sits under the tab bar on every tab, since loop state governs whether the BLM/INT grids are accumulating. **Session UX keys** — every file-producing action opens an inline filename prompt (default `goaldl_<ts>`, exclusive-create: a collision keeps the prompt open, never overwrites): `s` saves all four grids (`_BLM/_INT/_O2/_SPARK.txt`), `c` clears the active grid or resets sensor MIN/MAX (spark keeps its knock baseline), `r` toggles live raw recording (`stream.RecordSink` — fail-soft: a write error detaches + notices, never kills the session; on replay it's a self-expiring no-op warning), `d` toggles a decoded-frame CSV log (ParseOK rows only, same format as `monitor -csv`), `space`/`+`/`-` pause and re-pace a replay (0.25×–16×, non-retroactive, via runtime `ReplayProvider` controls; no-op on live). The model runs the session in a goroutine and receives `Snapshot`s over a channel; decoded views render from the last ParseOK snapshot (a bad sample only updates the raw view and the red/green footer heartbeat — WinALDL-style gating, a consumer-level decision per the data policy). INT/O2/spark grids and extrema are **consumer-side accumulation** over the same `Snapshot` stream (no Snapshot/Session/ecm change; spark differences the parsed cumulative `knock_count` mod 256, first frame = baseline). View rendering reuses the pure builders in `pkg/stream` (`SensorTable`/`SensorTableExtrema`, `BLMBody`/`INTBody`/`O2Body`/`SparkBody` over a shared `gridHeat`, `LoopStatus`, `FlagsBody`, `CodesBody`, `RawHistory`). Model logic is unit-tested in `tui_test.go` (incl. a full-fixture end-to-end run). WinALDL-parity roadmap: `specs/2026-07-04_feature_winaldl-parity/` — Phase 1 (diagnose) shipped 2026-07-04; Phase 2 (tune) shipped + verified 2026-07-04; Phase 3 (session UX: recording/CSV toggles, replay pause/speed keys, spark grid, filename prompts) shipped + verified 2026-07-04; next: a `serve` adapter proving the `Session` API drives a non-terminal front-end (Phase 4 items deferred).
- `pkg/blm/` - BLM (Block Learn Multiplier / long-term fuel trim) accumulator: bins readings into an RPM × MAP grid and emits Samples/Average/Correction tables (matching `data/20250601_162123_BLM.txt`). The tuning metric is **Wide Average** — the mean BLM over every valid sample in a cell; target 128, >128 = lean, <128 = rich. Correction = avg/128 (multiply base VE/fuel by it). Frame→sample extraction + closed-loop/BLM-enable gating + MAP-volts→kPa live in `pkg/ecm/fueltrim.go` (`FuelTrimSample`, `MapVoltsToKPa`), shared by the `blm` command and the live view. The MAP transfer is **verified against WinALDL** (2026-07-04): kPa = (raw + 28.06) / 2.71, exact across the ground-truth log's raw range 49–190; with it our drive-capture BLM cells match WinALDL's own table (e.g. 1600 RPM × 40 kPa: 117.17 vs WinALDL's 117.5). (The reference file's Narrow + Avg10/StdDev variants are intentionally not built — Wide Average is the metric used for tuning.)
- Live BLM view: `monitor -blm` streams frames through `pkg/stream`'s `BLMView`, which drives the same `pkg/blm.Grid`, redraws a compact heatmap in place on a TTY (active cell reverse-highlighted, closed/open-loop status, active-cell progress `n/min`), and prints the final Average + Correction tables on exit.
- **Confidence threshold** (`blm.DefaultMinSamples` = 4, `-min` flag on both `blm` and `monitor -blm`): BLM hunts, so a cell isn't trusted until it has enough readings (WinALDL uses ~3-4). Below the threshold a live cell renders dim (accumulating) and its correction is held at 1.000 (no change); at/above it renders solid and its correction is applied. `Grid.CorrectionAtLeast(min)` and `Grid.PopulatedCells(min)` implement this.
- `pkg/ecm/` - ECM definitions and frame parsing (GM 1227747 per A033.ads; byte order verified against the WinALDL log). Everything is data, the parser stays generic: sensor conversions (`Parameter` Factor/Bias/Lookup + dual-unit `Alt`), status-word bits (`FlagWords` — MW2/MWAF1/MCU2IO, `DecodeFlags`), trouble codes (`ErrorCodes` — MALFFLG1-3, `DecodeCodes`), per-byte labels (`ByteLabels`). Bit order is plain LSB-first, verified three ways (A033.ads `btBitNumber`, WinALDL log column order, live log rows). TPS % calibration via `WithTPSCalibration` (copy-on-write; registry definitions stay pristine). When reading `data/A033.ads`: its byte numbers are 1-based (ADS byte N = frame offset N−1).
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
6. ✅ **WinALDL PARITY PHASE 1 (2026-07-04)**: flags/error-codes/knock decoded as ecm data tables; Snapshot carries Flags/Codes; TUI grew to 5 tabs (Flags, Codes, scrolling raw history) with heartbeat + bad-sample gating; sensor table dual-unit (TPS % calibrated via -tps0/-tps100, MAP kPa); `MapVoltsToKPa` corrected to the WinALDL-verified transfer. Delta analysis + phased plan: `specs/2026-07-04_feature_winaldl-parity/`.
7. ✅ **WinALDL PARITY PHASES 2+3 (2026-07-04)**: Phase 2 (tune — INT/O2 grids, in-TUI save/clear, sensor Min/Max, persistent loop chrome) and Phase 3 (session UX — `r` raw-recording toggle via `stream.RecordSink`, `d` CSV toggle, replay pause/speed keys via runtime `ReplayProvider` controls, spark-counts grid on WinALDL axes, filename prompt on all file actions) both shipped and independently verified. Next: `serve` adapter (HTTP/WebSocket over the same Snapshot stream); Phase 4 deferred items (Dash view, config persistence, multi-ECM). VSS/vehicle-speed reads 0 on this vehicle — either not wired to the ECM or was captured stationary; not a decoder issue (byte 5 is genuinely 0x00). Known accepted divergence: WinALDL's coolant curve is smooth and reads ~3°F below our stepped A033 table at warm idle.
8. Optional phase 2 (onboard datalogging): MCU bridge per Hardware section below.
