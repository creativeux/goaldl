# goaldl ALDL bridge — ESP32-S3 firmware + hardware notes

A WiFi/TCP bridge that forwards the raw ALDL UART byte stream to `goaldl -tcp host:port`.
Board: Adafruit QT Py ESP32-S3 (No PSRAM) running CircuitPython 10.x. The bridge is a
byte pipe — no framing, no filtering, no timing (raw-data policy); goaldl's decoder finds
frame sync itself.

## Files

- `code.py` — the firmware. Copy to the board's `CIRCUITPY` drive.
- `settings.toml` — configuration **template** (safe defaults, no secrets). Copy to the
  drive and edit there; WiFi credentials live only on the board, never in this repo.

## Configuration (`settings.toml` on the board)

| Key | Default | Meaning |
|---|---|---|
| `BRIDGE_MODE` | `"ap"` | `"ap"` = create the `goaldl` WiFi network (car use); `"sta"` = join an existing network (bench use, or the phone's Personal Hotspot in the field) |
| `BRIDGE_SSID` / `BRIDGE_PASSWORD` | `goaldl` / `aldl1227` | AP credentials to create, or network to join |
| `BRIDGE_PORT` | `3333` | TCP listen port |
| `BRIDGE_TEST` | `"1"` | `1` = serve synthetic, correctly-encoded 1227747 idle frames at the real ~160 B/s (validates the whole WiFi/TCP/decoder path with zero wiring); `0` = read the real UART |

Status LED: yellow = starting · red/yellow blink = station-mode join failing (bad
password or network out of range; retries every 3 s) · blue = up, waiting for a client ·
green = client connected · red blink = client dropped. (Requires the `neopixel` library;
the firmware runs fine without it, just dark.)

**Known limitation (station mode):** the join retry covers startup only — if the joined
network drops mid-session, the bridge needs a power cycle. Irrelevant in AP mode (the
default in the car), where the bridge *is* the network.

## Validation log

- **2026-07-18 — WiFi/TCP leg proven** (no wiring): `BRIDGE_TEST=1`, station mode on the
  bench LAN; `goaldl monitor -tcp <ip>:3333` decoded the synthetic stream with PROM 6291
  matching (`prom_ok=true`), correct idle sensor values, and ~1.17 s frame cadence
  (real ECM: ~1.18 s). Desktop side: `stream.TCPProvider`, merged as PR #42
  (`specs/2026-07-06_feature_tcp-provider/`).
- **2026-07-19 — review-hardening pass** (agent PR review findings): client sends are
  bounded (`settimeout(2)`) so a wedged-but-open peer degrades to a normal client-drop
  instead of stalling the loop for the OS's TCP retransmission timeout; station-mode
  join retries in a loop (red/yellow blink + console message on failure) instead of
  halting on a wrong password or out-of-range network; `TestSource.read(n)` honors the
  caller's byte cap. Code-reviewed; on-board smoke re-run pending next replug.
- **Pending — real-UART bench leg**: `BRIDGE_TEST=0`, a 3.3V USB-TTL adapter replays
  `pkg/decoder/testdata/drive_4800.raw` at 4800 baud into the RX pin (TX→RX, GND→GND);
  expect 635/635 frames over WiFi.
- **Pending — car leg**: input-conditioning stage (below) on ALDL pins E/A.

## Input conditioning (car wiring)

The QT Py's pins are 3.3V-max; the ALDL line lives in the car's 12V domain (the data
line itself typically idles ~5V, but design for the dirty case). One NPN stage clamps
and inverts — the same two jobs the PL2303 cable's internal circuit does, so the bytes
match a serial capture byte-for-byte:

```
ALDL pin E ── R1 10kΩ ──┬── base  Q1 2N3904
                        │
             (optional R3 100kΩ base→GND)

QT Py 3V ─── R2 10kΩ ───┬── QT Py RX
                     collector
                      emitter
                        │
ALDL pin A ─────────────┴── QT Py GND   (shared ground)
```

- Line high → Q1 on → RX low; line pulsed low → Q1 off → R2 pulls RX to 3.3V. Switches
  at ~0.7V. R1 makes 12V+ transients on the line a non-event.
- Polarity is insurance-covered either way: `goaldl -tcp <addr> -invert` flips the
  decoder if a build comes out non-inverted.
- Optocoupler alternative (PC817 + ~1kΩ) buys galvanic isolation, same behavior.
- **Measure first**: key on, engine off — pin E to pin A should read a few volts and be
  busy. 12-pin connector, top row F E D C B A / bottom G H J K L M (no pin I); the
  PL2303 cable taps the same E + A.
- Power in the car: 12V accessory → USB adapter → USB-C. USB is power-only when
  deployed; data leaves over WiFi. (On the bench, USB also carries the CircuitPython
  console and the `CIRCUITPY` drive.)

## One connector for all ECM generations (design decision, 2026-07-18)

Goal: a single physical connector/interface design covering both ALDL generations, so
one built cable serves every use case. This works as a **superset**, not a compromise —
the generations differ in signal, not connector:

| | 160-baud (this project's target) | 8192-baud (mid-'86+) |
|---|---|---|
| Connector | same 12-pin shell | same 12-pin shell |
| Data pin | E | M |
| Signal | one-way PWM broadcast | half-duplex UART, request/response |
| Input stage | inverting NPN clamp (above) | non-inverting clamp (or ESP32 RX-invert in hardware) |
| TX path | none needed | one open-collector NPN driving the line |
| ESP32 side | UART0 RX @ 4800 (the UART-sampling trick) | second UART @ 8192 (S3 has 3) |

So the universal build wires **both** pin E and pin M, each through its own small stage,
to two different UARTs; firmware/config selects which is live. Hardware delta over the
160-baud-only build: roughly two more transistors and a handful of resistors.

**Documented limitation:** 8192-baud support is a hardware-ready door, not a working
feature. goaldl's decoder is purpose-built for the 160-baud pulse-width scheme;
8192 needs a second decode path (standard UART framing, mode-request/response protocol,
checksums, per-ECM frame tables — the Horizon 3 ADX-import work is how those
definitions would arrive). Build the universal connector; ship the 160-baud feature.

## Wiring diagram

[`wiring.html`](wiring.html) is the rendered schematic sheet — voltage-domain schematic,
signal waveforms, connector pinout, bench variant, generation-comparison table, and the
parts list. Self-contained HTML (no external assets, light/dark aware): open it in any
browser. The ASCII schematic above is the quick in-terminal version of the same circuit.
Parts: Q1 2N3904 (any small NPN), R1/R2 10kΩ, R3 100kΩ optional.
