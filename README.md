# goaldl

[![CI](https://github.com/creativeux/goaldl/actions/workflows/ci.yml/badge.svg)](https://github.com/creativeux/goaldl/actions/workflows/ci.yml)

**goaldl** reads live data from GM's pre-OBD2 diagnostic port — the 160-baud
ALDL (Assembly Line Diagnostic Link) stream — and shows it on an interactive
dashboard right in your terminal: sensors, fuel-trim maps, status flags, and
trouble codes. One small binary, no extra software, on macOS, Windows, or Linux,
using an ordinary USB-to-ALDL cable. Primary target: the **GM 1227747** ECM
(A033 TBI, '86–'88 4.3/5.0/5.7L).

![GoALDL dashboard demo](docs/goaldl-demo.gif)

It's working and validated on real hardware — a real 14-minute drive decoded
with every one of 635 frames matching the ECM's PROM ID across the full
operating range.

## What you need

- **A compatible GM vehicle** with an ALDL port — usually a 12-pin connector
  under the dash.
- **A USB-to-ALDL cable** — an inverting adapter onto the ECM's data line. A
  Prolific PL2303 or a genuine FTDI FT232R both work. See
  [Hardware & drivers](docs/usage.md#hardware--drivers) for the driver each OS
  needs.

## Install

**Download a prebuilt binary** (easiest) — grab the one for your OS from the
[Releases](https://github.com/creativeux/goaldl/releases) page and run it.

<details>
<summary><b>macOS:</b> allow the downloaded binary</summary>

The prebuilt binaries aren't yet code-signed, so macOS Gatekeeper blocks a
freshly downloaded copy — *"Apple could not verify 'goaldl' is free of
malware…"*. That's just the quarantine flag macOS adds to anything downloaded
from a browser. Clear it once, then run:

```bash
xattr -d com.apple.quarantine ./goaldl   # or: xattr -cr ./goaldl
./goaldl
```

Or right-click the binary in Finder → **Open**, or after a blocked launch open
**System Settings → Privacy & Security** → **Open Anyway**. Building from source
avoids this entirely (a locally built binary is never quarantined).
</details>

**Or build from source** — needs [Go](https://go.dev/dl/) 1.26+:

```bash
git clone https://github.com/creativeux/goaldl.git
cd goaldl
go build ./cmd/goaldl     # produces ./goaldl in the current folder
```

## Use it

You don't need to install it anywhere. Run the binary from wherever it is:

- **`./goaldl`** — a downloaded or freshly built binary, from its folder.
- **`go run ./cmd/goaldl`** — straight from a source checkout, no build step.
- **`goaldl`** — if you've put it on your PATH (optional; handy if you'll use it
  a lot — e.g. `sudo mv goaldl /usr/local/bin/` on macOS/Linux).

The examples below write it as `goaldl` for brevity — use whichever form above
fits you.

Plug in the cable, find your port, and launch the dashboard:

```bash
goaldl ports              # list serial ports — find your adapter
goaldl -p /dev/cu.usbserial-10   # launch the dashboard, live from the car
goaldl                    # or just this, if only one adapter is plugged in
```

Switch tabs with the number keys or `tab`; press `q` to quit. From here you can
save fuel-trim grids, record the session, and more.

**No car handy?** Try it on canned data — no hardware needed:

```bash
goaldl simulate -n 200 && goaldl aldl_sim_4800.raw   # synthetic stream
```

## Documentation

- **[Usage guide](docs/usage.md)** — the dashboard tabs and keys, recording a
  drive at the car, the scripting/headless commands, and hardware & driver notes.
- **[BLM fuel-trim tuning](docs/blm-tuning.md)** — turn a drive capture into a
  rich/lean fuel map.
- **[How it works](docs/protocol.md)** — the ALDL protocol and how goaldl
  decodes it.
- **[Development](docs/development.md)** — building, testing, releases, and the
  project layout.

## License

Copyright 2026 Aaron Stone. Released under the
[PolyForm Noncommercial License 1.0.0](LICENSE): free to use, modify, and share
for any **noncommercial** purpose — personal projects, hobby use, research,
education, and nonprofits.

**Commercial use** — selling goaldl, or a product or service built on it —
requires a separate commercial license from the author. If that's you, get in
touch.
