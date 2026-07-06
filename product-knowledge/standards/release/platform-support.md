# Platform support matrix

**Standard:** goaldl ships as a single pure-Go codebase (`CGO_ENABLED=0`, no
build tags outside `cmd/goaldl/vt_*.go`) that cross-compiles for every tier
below. Platform work happens in exactly two places — the serial layer
(`pkg/serial`, port naming + drivers) and the terminal (ANSI/VT handling) —
so keep OS-conditional code confined to those seams.

Audited 2026-07-06: `go build ./...` verified clean for darwin/amd64+arm64,
linux/amd64+arm64+arm+riscv64, windows/amd64+arm64, freebsd/amd64, and
openbsd/amd64. Hardware assumption throughout: an inverting USB-to-ALDL cable
on a PL2303 (tested) or FTDI FT232R (fallback) adapter.

## Tier 1 — Core (built, tested, supported)

| Platform | Arch | OS minimum (Go 1.26 floor) | PL2303 driver | Port names |
|---|---|---|---|---|
| macOS | arm64, amd64 | macOS 12 Monterey | Prolific "PL2303 Serial" DriverKit app (App Store); pre-2012/counterfeit HXA chips are driver-blocked | `/dev/cu.usbserial-*`, `/dev/cu.PL2303-*` |
| Windows | amd64 | Windows 10 | Auto-installs via Windows Update; same HXA counterfeit block | `COM*` |
| Linux (glibc and musl — static binary) | amd64, arm64 | kernel ≥ 3.2, any distro | `pl2303` module is in-tree everywhere; user must be in the `dialout` (Debian/Ubuntu/Pi OS) or `uucp` (Arch) group | `/dev/ttyUSB*`, `/dev/ttyACM*` |

macOS on Apple Silicon is the only hand-tested platform as of the first
release; the other Tier 1 rows are supported targets we accept bug reports
for and fix before release-blocking issues ship.

## Tier 2 — Built and expected to work (best-effort)

| Platform | Arch | Notes |
|---|---|---|
| Raspberry Pi 3/4/5 (64-bit Pi OS) | linux/arm64 | Covered by the standard Linux build. In-tree pl2303 driver; TUI works over SSH. Pi 5's pigpio limitation only affects a future GPIO-direct bridge, not USB adapters |
| Raspberry Pi Zero/1/2 (32-bit Pi OS) | linux/arm (GOARM=6) | Dedicated GoReleaser target |
| Windows | arm64 | Binary ships; Prolific's ARM64 driver support is spotty — recommend the FTDI fallback adapter |
| FreeBSD | amd64 | `uplcom(4)` driver in base; ports enumerate as `/dev/cuaU*` |

Tier 2 targets are produced by every release but are not hand-tested;
regressions are fixed as reported, not hunted.

## Tier 3 — Embedded (future, not in releases)

| Target | Verdict | Delta |
|---|---|---|
| Arduino-class ARM boards (RP2040/Pico, Nano 33, ESP32) via TinyGo | Core-only port is feasible | `pkg/decoder` imports only `math/bits`; `pkg/ecm`/`pkg/blm` use only `fmt`/`sort`/`strings` — all TinyGo-supported. Needs one new firmware `main` reading `machine.UART` at 4800 baud into the existing decoder, plus a TinyGo CI job. `pkg/serial`, `pkg/stream`'s Session plumbing, and Bubble Tea do not come along |
| Classic Arduino (Uno, 8-bit AVR) | Not viable | 2 KB RAM is below TinyGo's practical floor for this code; would be a C rewrite |
| Particle Photon 2 (RTL8721DM) | Not a port target | No Go or TinyGo backend for the SoC. Its only role is the planned C++ Device OS edge-timing *bridge* feeding a goaldl instance elsewhere — zero code sharing |

## Rules this implies

- **Never introduce CGO or an OS-conditional dependency into
  `pkg/decoder`/`pkg/ecm`/`pkg/blm`** — their freedom from heavy imports is
  what keeps the TinyGo door open and the release matrix cheap.
- New USB-serial chipset detection goes in `pkg/serial.filterUSBPorts` (unit
  tested; the CH340 prefix bug shipped precisely because the old inline
  filter was untestable).
- Anything that writes raw ANSI outside Bubble Tea must gate on
  `isTTY`/`enableVT` (`cmd/goaldl/vt_*.go`) so legacy Windows conhost gets
  plain sequential output instead of escape garbage.
- Release targets live in `.goreleaser.yaml` and must stay in sync with the
  tiers above; a new tier-1/2 row means a new build target in the same
  change.
