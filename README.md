# goaldl

A cross-platform Go scanner and datalogger for GM's 160-baud ALDL (Assembly
Line Diagnostic Link) protocol — the pre-OBD2 diagnostic stream used by GM
ECMs. Primary target: the **GM 1227747** (A033 TBI, '86–'88 4.3/5.0/5.7L).

**Status:** working and validated on real hardware. A byte-value decoder
reads the 160-baud stream through an ordinary USB serial adapter and produces
correct frames — verified against a WinALDL ground-truth log and a real drive
capture (635/635 frames with matching PROM ID across the full operating range).

## How it works, in one paragraph

The ALDL line idles high and encodes each bit as the *width* of a low pulse
(short ≈ logic 0, long ≈ logic 1). An inverting interface cable feeds this to
a PC UART, which frames exactly **one byte per ALDL bit** — so decoding is a
matter of reading byte *values*, not host-side timing (which USB makes
unreliable). At 4800 baud a short pulse arrives as `0xFE` and a long pulse as
`0x00`; nine consecutive 1-bits are the `0x1FF` sync that delimits 20-byte
frames. The full model and the history of getting here are in
[`CLAUDE.md`](CLAUDE.md); the implementation is in `pkg/decoder/`.

## Install

```bash
go build ./cmd/goaldl      # build
go run ./cmd/goaldl        # or run directly; prints available commands
```

## Commands

```bash
# Find the adapter (the device name drifts — check before using -p)
goaldl ports

# Capture raw bytes to a file (do this first at the car), then decode offline
goaldl record -p /dev/cu.usbserial-10 -t 60 -o drive_4800.raw
goaldl decode drive_4800.raw -o frames.csv

# Live sensor table straight from the vehicle — optionally record raw and log decoded CSV
goaldl monitor -p /dev/cu.usbserial-10 -o session.raw -csv live.csv

# Replay a capture as a live-updating table
goaldl monitor drive_4800.raw

# Build a BLM fuel-trim table (where the tune runs rich/lean) from a capture
goaldl blm drive_4800.raw -o correction.csv

# Watch the BLM grid fill live while driving (records raw at the same time)
goaldl monitor -p /dev/cu.usbserial-10 -blm -o session.raw

# Generate a synthetic capture to exercise the decoder without hardware
goaldl simulate -n 10 && goaldl decode aldl_sim_4800.raw

# List supported ECMs
goaldl ecms
```

The recommended workflow is **record then decode**: capture raw bytes once at
the car, then re-run `decode`/`monitor`/`blm` offline against that file as many
times as you like. For live use, `monitor -p` shows the table and can record raw
(`-o`) and log decoded CSV (`-csv`) at the same time.

## BLM fuel-trim tuning

The `blm` command turns a drive capture into a fuel-trim map — a picture of
where the base tune runs rich or lean across RPM and load. It reads the Block
Learn Multiplier (BLM, the ECM's long-term fuel trim) from each frame and bins
it into an RPM × MAP grid.

**Reading BLM: 128 is neutral.** Below 128 the ECM is *removing* fuel because
the mixture ran rich (the base tune has too much fuel there); above 128 it is
*adding* fuel because it ran lean. The **correction factor** each cell reports
is `avg_BLM / 128` — multiply that cell's base VE/fuel by it to move the ECM
back toward 128.

Only closed-loop, block-learn-enabled frames are recorded — BLM is frozen and
meaningless at wide-open throttle, on decel, or before warm-up, so those frames
are skipped. A cell also isn't trusted until it has collected enough readings
(default 4; BLM hunts, so one or two samples are noisy). Below that threshold a
cell's correction is held at `1.000` (no change) and, in the live view, drawn
dim while it accumulates.

```bash
# Offline: build the tables from a capture, write the correction grid to CSV
goaldl blm drive_4800.raw -o correction.csv
goaldl blm drive_4800.raw -min 3      # trust a cell at 3 samples (WinALDL-like)

# Live: watch each cell fill and settle as you drive (· = empty, dim =
# accumulating, solid = trusted; the active cell is highlighted)
goaldl monitor -p /dev/cu.usbserial-10 -blm -o session.raw
```

`blm` prints three tables — Samples, Wide Average BLM, and the Correction
factor — matching the format of `data/20250601_162123_BLM.txt`. The MAP→kPa
axis uses a standard GM 1-bar transfer (`pkg/ecm/fueltrim.go`); it only affects
which column a reading lands in, not the BLM math.

## Project layout

```
cmd/goaldl/            CLI: main.go (ports/ecms) + capture.go (record/decode/simulate)
                       + monitor.go (live table + -blm grid) + blm.go + csv.go
pkg/decoder/           The decoder (byte-value state machine) + synthetic encoder + tests
    testdata/          Real raw captures + golden frame dumps — the root of the test suite
pkg/stream/            Provider abstraction (replay + serial) feeding the live sensor table + BLM grid
pkg/blm/               BLM fuel-trim accumulator (RPM × MAP grid, averages, correction)
pkg/ecm/               ECM definitions, frame parsing, and fuel-trim extraction (GM 1227747 per A033.ads)
pkg/serial/            Thin serial-port wrapper (open/read/flush/list) — no decoding
pkg/aldl/              Shared Frame type
pkg/errors/            Error types
data/                  Reference captures and A033.ads ECM definition
docs/history/          Superseded debugging notes, kept for context
```

## Testing

```bash
go test ./...
```

The suite is rooted in real captures under `pkg/decoder/testdata/`:
`TestDecodeRealCapture` asserts exact decode stats and 100% PROM-ID match on
the idle and drive recordings, and `TestGolden` pins the exact decoded frame
bytes. After an intentional decoder change, regenerate the golden files with:

```bash
go test ./pkg/decoder -run TestGolden -update   # then review the diff before committing
```

## Data policy

The decoder is a faithful transport: it does **no** plausibility filtering,
outlier rejection, or smoothing, and emits every structurally-aligned frame
as-is (warts included). Quality signals ride alongside the data (e.g. PROM-ID
match); data-quality decisions belong to downstream consumers where they can be
tuned or disabled.

## Hardware

- A USB-to-ALDL cable/adapter (an inverting level converter to the UART RX
  line). Tested with a Prolific PL2303 on macOS; a genuine FTDI FT232R is a
  good alternative (native driver on macOS). See [`CLAUDE.md`](CLAUDE.md) for
  driver notes and an onboard-MCU option.
- A compatible GM vehicle with an ALDL port (typically under the dash).

## References

- ALDL 160-baud spec: <https://www.techedge.com.au/vehicle/aldl160/160serial.htm>
- Decoding GM ALDL with a Teensy: <https://www.bot-thoughts.com/2018/01/decoding-gms-aldl-with-teensy-36.html>
- A033.ads ECM definition: `data/A033.ads`

## License

GPL-3.0 (maintains compatibility with the original rustaldl project).
