# goaldl ALDL bridge — ESP32-S3 firmware + hardware notes

A WiFi/TCP bridge that forwards the raw ALDL UART byte stream to `goaldl -tcp host:port`.
Board: Adafruit QT Py ESP32-S3 (No PSRAM) running CircuitPython 10.x. The bridge is a
byte pipe — no framing, no filtering, no timing (raw-data policy); goaldl's decoder finds
frame sync itself.

## Files

- `code.py` — the firmware. Copy to the board's `CIRCUITPY` drive.
- `settings.toml` — configuration **template** (safe defaults, no secrets). Copy to the
  drive and edit there; WiFi credentials live only on the board, never in this repo.
- `bench.py` — bench/diagnostic tool, not part of the firmware. Copy it alongside
  `code.py` and run `import bench` from the REPL: it prints the raw logic level on the
  UART RX pin twice a second. Statically verifies the input-conditioning stage with no
  signal source, and answers "is RX even toggling?" first thing in the car. Only one of
  `bench.py` / `code.py` can own `board.RX` at a time.

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
- **2026-08-29 — input-conditioning stage proven statically** (no adapter, no car): the
  NPN stage below, built on a breadboard; `bench.py` reads `board.RX` while the stage
  input (the ALDL-pin-E side of R1) is jumpered to 3V and to GND in turn. Measured
  `3V → RX LOW`, `GND → RX HIGH`, `floating → RX HIGH` — clamp + invert confirmed, and
  with it Q1's orientation, the emitter ground, and the R1/R2/R3 placements. **Not**
  covered: switching speed (static levels only — though a 2N3904 switches in well under
  1 µs against a 6250 µs bit cell) and the 12V clamp itself (nothing exceeded 3.3V).
  *Superseded 2026-08-30: this measured exactly what the spec asked for, and the spec
  was wrong — a single inverting stage. The circuit was faithfully built and faithfully
  tested against a backwards requirement, which is why the static check passed and the
  car did not. See the entry below.*
- **2026-08-30 — polarity fault found and fixed; CAR LEG PROVEN.** First car attempt
  (breadboard, pins E/A, engine running) reached the dashboard's `waiting for frame
  sync` state — bytes arriving, no sync. A 60 s raw capture via
  `monitor -tcp 192.168.4.1:3333 -o` settled it in one histogram: **99.7% `0x00`, zero
  `0xFE`**, with `0x00` runs up to 1587 — a UART held in continuous break, i.e. RX
  idling low. `-invert` failed too (as it must: framing artifacts, not inverted data),
  as did 2400 baud. Root cause: the single NPN stage inverts, but the ALDL line is
  already UART-shaped and RX must follow pin E directly. Fix: a **second identical NPN
  stage** in series (Q2/R4/R5, RX moved to Q2's collector). Re-verified statically
  (`3V → HIGH`, `GND → LOW`, `floating → LOW`) then in the Jeep: **70.7% `0xFE` /
  29.2% `0x00`, 99.85% clean, 32/32 frames, PROM ✓**, warm idle at RPM 825 / coolant
  176 °F / batt 13.5 V / BLM 125 / INT 128. The bridge is proven end to end on a
  running engine.
- **Pending — real-UART bench leg**: `BRIDGE_TEST=0`, a 3.3V USB-TTL adapter replays
  `pkg/decoder/testdata/drive_4800.raw` at 4800 baud into the RX pin (TX→RX, GND→GND);
  expect 635/635 frames over WiFi. Now a regression fixture rather than a gate — the car
  leg has already proven the path.
- **Pending — perfboard build**: the breadboard is proven but is not a vehicle install.
  Transfer to soldered perfboard with strain relief before any datalogging drive.

## Input conditioning (car wiring)

The QT Py's pins are 3.3V-max; the ALDL line lives in the car's 12V domain (the data
line itself typically idles ~5V, but design for the dirty case). The input stage must
clamp that domain away from the pin **while preserving polarity** — see the polarity
note below, it is the part that is easy to get wrong. Two NPN stages in series: the
first clamps and inverts, the second inverts back.

Each stage is three nodes — base, collector, emitter — drawn one stage at a time so
every connection is explicit:

```
STAGE 1 — clamps the car's domain off the board, and inverts

  ALDL pin E ──[R1 10kΩ]──┬─────────────── Q1 base
                          └──[R3 100kΩ]─── GND            (optional)

  QT Py 3V ────[R2 10kΩ]──┬─────────────── Q1 collector
                          └─────────────── into stage 2 ──┐
                                                          │
                                           Q1 emitter ─── GND

STAGE 2 — inverts back, so RX ends up following pin E     │
                                                          │
  stage 1 out ─[R4 10kΩ]──┬─────────────── Q2 base ◄──────┘

  QT Py 3V ────[R5 10kΩ]──┬─────────────── Q2 collector
                          └─────────────── QT Py RX

                                           Q2 emitter ─── GND

  ALDL pin A ───────────────────────────── QT Py GND   (shared, required)
```

- **Stage 1**: line high → Q1 on → its collector low. Line pulsed low → Q1 off → R2
  pulls the collector to 3.3V. Switches at ~0.7V; R1 limits base current, so a *positive*
  12V transient forward-clamps harmlessly through the base-emitter junction. A *negative*
  spike is not covered — Q1's V(EBO) is only ~6V — so add a base-to-ground diode if the
  install sees a dirty line (`docs/mobile-ui.md` makes the same point).
- **Stage 2**: repeats the inversion, so RX ends up following pin E directly — idle
  high, pulsing low, which is what a UART needs.
- **RX comes off Q2's collector only.** Nothing connects Q1's collector to RX; that is
  the single most common slip when rebuilding from the one-stage version.

### Polarity: why two stages and not one

A UART start bit is a falling edge from an idle-high line. The ALDL line is already
UART-shaped (idles high, pulses low), so **RX must follow pin E directly** — a short
low pulse becomes `0xFE`, a long one `0x00`. A single inverting stage holds RX *low*
between pulses, which the UART reads as a continuous break.

That failure is unmistakable in the data, and `goaldl -invert` does **not** rescue it —
the bytes are framing artifacts, not inverted data, so there is nothing to flip:

| Capture | `0xFE` | `0x00` |
|---|---|---|
| Healthy (`pkg/decoder/testdata/idle_4800.raw`) | 69.4% | 30.5% |
| Healthy (this bridge, 2026-08-30) | 70.7% | 29.2% |
| **One inverting stage — broken** | **0%** | **99.7%** |

**Check the byte mix before anything else** when a build won't sync. `monitor -o` a
minute of raw bytes and histogram it; the answer is immediate and needs no hardware.

What the byte values actually pin down is that there is **no net inversion** between
pin E and the UART's RX input. How a given cable achieves that is its own business: the
classic ALDL cables contain one inverting transistor stage because they fed RS-232
receivers, which invert again — two inversions, net non-inverting, the same result this
two-NPN stage reaches. **A stage that inverts must be paired with a receiver that
inverts back.** An RS-232 line receiver does; a TTL UART pin like the QT Py's does not,
which is why this build needs stage 2. An earlier revision of this file specified a
single inverting stage feeding a TTL pin, which is the combination that cannot work.
`docs/mobile-ui.md` had it right all along ("Polarity is already UART-shaped at logic
level ... no inversion needed").

- Optocoupler alternative (PC817 + ~1kΩ) buys galvanic isolation — but an opto inverts,
  so it replaces stage 1 and still needs stage 2 (or an ESP32 with `UART_RXD_INV`
  exposed; CircuitPython's `busio.UART` does not expose it, which is why this build
  re-inverts in hardware).
- **Measure first**: key on, engine off — pin E to pin A should read a few volts and be
  busy. 12-pin connector, top row F E D C B A / bottom G H J K L M (no pin I); the
  PL2303 cable taps the same E + A.
- Power in the car: 12V accessory → USB adapter → USB-C. USB is power-only when
  deployed; data leaves over WiFi. (On the bench, USB also carries the CircuitPython
  console and the `CIRCUITPY` drive.)

### Static bench check (no adapter, no car)

With `bench.py` running, jumper the stage input (the pin-E side of R1) and watch RX.
The two-stage build does **not** invert, so:

| Stage input | RX |
|---|---|
| 3V | HIGH |
| GND | LOW |
| floating (R3 fitted) | LOW |

A one-stage build gives the opposite table — which is how a wrong build passes a static
test that was written against the wrong spec. Confirm against this table, then confirm
against the byte mix in the car.

## One connector for all ECM generations (design decision, 2026-07-18)

Goal: a single physical connector/interface design covering both ALDL generations, so
one built cable serves every use case. This works as a **superset**, not a compromise —
the generations differ in signal, not connector:

| | 160-baud (this project's target) | 8192-baud (mid-'86+) |
|---|---|---|
| Connector | same 12-pin shell | same 12-pin shell |
| Data pin | E | M |
| Signal | one-way PWM broadcast | half-duplex UART, request/response |
| Input stage | two-NPN non-inverting clamp (above) | the same clamp — net non-inverting, however it is built |
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
Parts: Q1 + Q2 2N3904 (any small NPN — check the marking, a 2N3906 is a PNP in an
identical package), R1/R2/R4/R5 10kΩ, R3 100kΩ optional.
