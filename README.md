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

# Live real-time decode straight from the vehicle
goaldl decode -p /dev/cu.usbserial-10 -o live.csv

# Generate a synthetic capture to exercise the decoder without hardware
goaldl simulate -n 10 && goaldl decode aldl_sim_4800.raw

# Parse / convert an existing hex capture file
goaldl test data/varied_sensors.hex
goaldl convert data/varied_sensors.hex -o output.csv

# List supported ECMs
goaldl ecms
```

The recommended workflow is **record then decode**: capture raw bytes once at
the car, then develop and re-run the decoder offline against that file as many
times as you like. `decode -p` is available for live monitoring.

## Project layout

```
cmd/goaldl/            CLI: main.go (ports/ecms/test/convert) + capture.go (record/decode/simulate)
pkg/decoder/           The decoder (byte-value state machine) + synthetic encoder + tests
    testdata/          Real raw captures + golden frame dumps — the root of the test suite
pkg/ecm/               ECM definitions and frame parsing (GM 1227747 per A033.ads)
pkg/serial/            Thin serial-port wrapper (open/read/flush/list) — no decoding
pkg/aldl/              Shared Frame type
pkg/logging/           CSV/JSON/raw/hex loggers
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
