# Mobile UI (iOS + Android) — Architecture & Transport Notes

*Research summary, 2026-07-06. Context: goal is a native mobile front-end for goaldl — plug in (or
connect wirelessly) at the car and see the live dashboard on a phone. This doc covers what ports,
what gets rewritten, and the three candidate transports for getting ALDL bytes into a phone. The
ESP32 option here is the concrete plan for the MCU-bridge idea already sketched in CLAUDE.md's
Hardware section.*

## The shared core ports to both platforms

The Go engine — `pkg/decoder`, `pkg/ecm`, `pkg/blm`, and `pkg/stream`'s `Session`/`Snapshot` — is
pure Go with no OS dependencies. `gomobile bind` compiles it to:

- **iOS**: an XCFramework called from Swift
- **Android**: an AAR called from Kotlin/Java

One engine, two thin native UIs. The Bubble Tea TUI does not port; each platform gets a native
front-end (SwiftUI / Jetpack Compose) rendering the same `Snapshot` stream — effectively the
Phase 4 `serve` adapter idea, with a language binding instead of HTTP.

Binding notes: `gomobile bind` restricts types across the boundary, so the cleanest surface is
`Session` + a **byte-push provider** (native side feeds raw bytes in, regardless of transport) +
`Snapshot` out as JSON or accessor methods. A small adapter package, not a rework. The byte-push
provider shape is transport-agnostic — BLE, MFi, and USB-OTG all reduce to "native code hands the
Go core a byte stream," so all three transports share one Go-side integration.

## The transport problem

The decoding model transfers to any transport: the adapter's only real job is letting *some
hardware UART* sample the ALDL pulse widths at 4800 baud — one UART byte per ALDL bit
(`0xFE` = 0, `0x00` = 1). Byte values are timing-independent (the insight that made USB buffering
irrelevant), so bursty wireless delivery is harmless. At ~160 bytes/sec, bandwidth is trivial on
every option below. Raw captures through any of these transports are **byte-for-byte
interchangeable** with PL2303 captures — existing `.raw` fixtures, golden tests, and replay
tooling remain the test suite unchanged, and the keep-raw-data-raw policy holds (every transport
is a faithful byte pipe; decode decisions stay downstream).

The platforms differ sharply in what they let an app open:

| Transport | iOS | Android | Desktop |
|---|---|---|---|
| Plain USB serial (PL2303/FTDI) | ✗ no public API | ✓ USB Host API + OTG | ✓ (today's path) |
| MFi serial cable (Redpark) | ✓ ExternalAccessory | ✗ (Apple-specific) | — |
| ESP32 bridge, BLE | ✓ CoreBluetooth | ✓ Android BLE | needs BLE support |
| ESP32 bridge, TCP (WiFi or wired Ethernet) | ✓ plain sockets | ✓ plain sockets | ✓ same `TCPProvider` |

Key facts behind the table:

- **iPhone cannot open an ordinary USB serial adapter.** iOS has no public user-space USB API for
  arbitrary devices (DriverKit-for-USB is iPadOS-only as of knowledge-cutoff Jan 2026); no
  PL2303/FTDI drivers exist. USB-C on iPhone 16 Pro hosts only Apple-blessed classes (storage,
  ethernet, audio, MIDI).
- **Android is the opposite:** the public USB Host API plus the mature `usb-serial-for-android`
  library (PL2303, FTDI, CH340, CP210x) means **the existing cable + a USB-C OTG adapter works on
  an Android phone today, no root required.** Android wired support is nearly free.
- **BLE is fully public and unrestricted on both platforms.**
- **TCP over any network interface is fully public too** — this is what commercial "WiFi OBD2"
  dongles (WiFi ELM327s) actually are: the dongle runs a WiFi AP and exposes a raw TCP socket the
  app connects to. No MFi involved. And iOS natively supports **standard USB Ethernet adapters**
  (one of the Apple-blessed USB classes on USB-C iPhones), so a bridge with an Ethernet jack gives
  a *wired* iPhone connection over CAT5 with zero MFi/vendor-SDK friction.
- Net: the bridge (Option A) covers iOS, Android, and desktop with a single accessory, and its
  TCP mode works over both WiFi and wired Ethernet with identical app-side code.

## Option A — ESP32 wireless bridge (recommended)

**The ESP32 is a wireless replacement for the PL2303, nothing more. No decoding in firmware.**

The ESP32 (~$5 dev board, three hardware UARTs, built-in BLE + WiFi) does exactly what the PL2303
does: UART at 4800 baud on the ALDL line, forward raw bytes over a link of choice. Firmware is
~50 lines of C/Arduino. The ESP32 UART has a built-in RX-invert flag (`UART_RXD_INV`) if the input
stage inverts.

**Link-layer variants** (same firmware skeleton, same byte stream, pick per session):

- **WiFi + TCP** — the bridge runs a WiFi AP and serves a raw TCP socket, exactly the commercial
  WiFi-OBD2-dongle model. Zero extra hardware. Known annoyance (shared by every WiFi OBD2 app):
  joining the bridge's AP captures the phone's WiFi, so internet routes over cellular while
  logging; the direct socket keeps working.
- **Wired Ethernet + TCP** — swap the plain ESP32 for a **WT32-ETH01** (~$10, ESP32 with LAN8720
  Ethernet PHY onboard) or add a W5500 SPI module; CAT5 from the bridge to a standard USB-C
  Ethernet adapter on the iPhone (natively supported device class — no MFi, no vendor SDK).
  Wired reliability without the captive-WiFi annoyance. Note Ethernet does not power the bridge
  (no PoE at this scale) — the 12V accessory power plan still applies — and with no router in the
  car the bridge should self-assign a link-local address and advertise via mDNS/Bonjour (good
  ESP32 support; native discovery on iOS), or just use a static IP.
- **BLE** — lowest-friction pairing UX, leaves phone WiFi free. Requires a BLE-specific transport
  layer per platform (CoreBluetooth / Android BLE) instead of plain sockets.
- **USB Ethernet gadget (single-cable, most elegant — experimental)** — an ESP32-S2/S3 (the
  variants with native USB) running TinyUSB can enumerate as a **CDC-NCM "USB Ethernet gadget"**,
  the same standard device class as commodity Ethernet adapters. One USB-C cable from the iPhone
  then provides both **power to the bridge** (iPhones source ~4.5W to accessories; the ESP32
  needs well under 1W) and the network link — no CAT5, no adapter, no 12V wiring, and the app
  still just opens the same TCP socket. Caveats: relies on iOS accepting a generic CDC-NCM
  gadget, which is community-verified (e.g. Linux USB-gadget devices on USB-C iPhones) but not
  Apple-documented and could shift across iOS versions; requires the S2/S3 specifically (rules
  out WT32-ETH01); the phone powers the bridge and can't charge while logging; and the firmware
  gains TinyUSB-NCM + link-local/DHCP setup. **Treat as a bench experiment, not the plan** — if
  it works it's the best form factor, but design around WiFi-TCP / Ethernet-TCP first.

**Power note:** plain Ethernet carries data only — PoE needs an injector and nothing in this
chain sources it; the phone's USB-C Ethernet adapter draws from the phone for itself but puts no
power on the cable. So except for the USB-gadget variant above, the bridge always needs its own
supply (the 12V accessory plan below). That's also the right default: a long logging session
shouldn't drain the phone to run the car interface.

Above the physical layer, WiFi-TCP and Ethernet-TCP are **the same transport**: one socket
protocol, one app-side code path, identical to the desktop validation path. BLE is a nice-to-have
after TCP works, not a prerequisite.

**Why it's the recommendation:** it is the only option that serves iOS, Android, and desktop with
one accessory and one code path; it offers both wireless convenience and a wired (Ethernet) mode
for reliability; and the TCP mode gives the **existing desktop `goaldl` wireless support before
any mobile app exists** — the cheapest way to validate the bridge with tooling we already trust.

**Go-side change:** one new `TCPProvider` beside `SerialProvider`/`ReplayProvider` (serves desktop
and, via the byte-push binding, mobile; a `BLEProvider` shape only if/when BLE is added), emitting
the same `FrameEvent` stream. Decoder/ecm/blm/Session untouched.
**Status: Stage 0 delivered** — `stream.TCPProvider` + the `-tcp host:port` source in the
dashboard and `monitor` shipped per `specs/2026-07-06_feature_tcp-provider/`; desktop `goaldl`
consumes any TCP byte source today. Next: bridge firmware bring-up on the bench (Stage 1).

**Why not run goaldl on the ESP32 itself:** standard Go doesn't target Xtensa (no bare-metal
RISC-V `GOOS` for the C3 either); TinyGo's ESP32 support is experimental with weak BLE/WiFi.
Porting would fork the decoder into a second C implementation on the hardest-to-debug device in
the chain, for no benefit.

**Optional later firmware mode:** the bot-thoughts edge-timing method (falling-edge interrupt,
sample ~2000μs later) as an alternative for ECM families whose pulse widths don't suit the UART
trick. Not needed for the 1227747.

**Cost:** it's a (small) hardware project — connector, conditioning circuit, enclosure, power.
Weekend-project tier; details in the Hardware section below.

## Option B — MFi serial cable (iOS wired)

Apple's ExternalAccessory framework lets an app talk to MFi-certified accessories, and Redpark
sells MFi-certified serial cables (Lightning and USB-C) aimed exactly at this niche, with
arbitrary baud rates including 4800. The UART trick works unchanged — it's still a hardware UART
sampling the pulses; whose UART doesn't matter.

**Pros:**

- No hardware project: buy the cable, write software. Fastest path to a working wired iPhone demo.
- Wired reliability — no pairing, no battery, no RF flakiness.
- Redpark's typical form factor is a DB9/RS-232 cable, so if the current ALDL interface presents
  RS-232 (the WinALDL-style circuit doing the ALDL↔RS-232 conversion), the Redpark cable may plug
  into the **same interface hardware**, just replacing the PL2303 leg. If the PL2303 is integrated
  into one sealed cable, the ALDL conditioning circuit has to be replicated — at which point the
  parts list starts resembling the ESP32 build anyway.

**Cons:**

- **iOS-only.** MFi/ExternalAccessory is an Apple program; it does nothing for Android. Choosing
  this as *the* transport forfeits the single mobile code path.
- ~$60–100 for the cable.
- Proprietary vendor SDK instead of POSIX serial: a Swift-side byte source feeding the Go core
  (same byte-push shape as BLE, so the Go side is identical — the lock-in is all in the Swift
  transport layer).
- App friction: the app's `Info.plist` must declare the accessory protocol string, and App Store
  distribution of MFi-consuming apps requires coordination with the accessory maker (Redpark has a
  process). Personal sideloading is unaffected.

**Verdict:** viable and legitimately attractive as a *fast first step* for an iOS-only wired
prototype, or as a wired fallback alongside BLE. Not sufficient alone for the mobile-UI goal,
because it strands Android.

## Option C — Android USB OTG (wired, nearly free)

Worth naming for completeness: today's PL2303/FTDI cable + a USB-C OTG adapter +
`usb-serial-for-android` in an Android front-end, feeding the same byte-push provider. No new
hardware beyond the ~$5 OTG adapter. *Not currently actionable for us (no Android device on
hand)*, but it means Android support falls out nearly free once a mobile app exists, and it's the
zero-hardware prototyping route for anyone who does have an Android device.

## Recommended path

*(iOS-first — no Android device available.)*

1. **Build the ESP32 bridge with WiFi-TCP** (Option A) and validate it against **desktop**
   `goaldl` via a `TCPProvider` — proves bridge + transport end-to-end with trusted tooling,
   before any mobile code exists.
2. **iOS app** (gomobile binding + SwiftUI + Network.framework socket) consumes the same TCP
   stream. Replay-driven development works here too: a dev-mode TCP server on the Mac replaying a
   committed `.raw` capture stands in for the car entirely.
3. **Add wired Ethernet** (WT32-ETH01 + USB-C Ethernet adapter) if the captive-WiFi annoyance or
   RF reliability warrants it — no app-side change, same socket.
4. **BLE mode** and/or a Redpark **MFi wired mode** (Option B) only if a concrete need emerges;
   both are additive transports over the same byte-push provider.
5. **Bench-test the USB-gadget single-cable variant** (ESP32-S3 as CDC-NCM device) whenever an
   S2/S3 board is on hand — if iOS accepts it, it supersedes the Ethernet mode as the wired form
   factor: one cable, phone-powered, same TCP socket.

## Hardware (ESP32 bridge)

### Vehicle connection — connector, don't solder

Never solder to the car. The 12-pin ALDL connector takes a repro mating plug (a few dollars);
spade/blade terminals work in a pinch. Needed contacts:

| Pin | Function | Notes |
|-----|----------|-------|
| A | Ground | Also the data reference |
| E | 160-baud serial data | Verify against the existing working cable before trusting |
| B | Diagnostic enable | **10kΩ resistor B→A** puts the ECM in ALDL mode (the working PL2303 cable almost certainly has this inside — open it / check its schematic and replicate) |

Keep the PL2303 cable fully intact as the known-good reference for A/B-ing captures.

### Signal conditioning — never wire pin E straight to a GPIO

ALDL data is 0–5V logic; ESP32 GPIO is 3.3V and **not 5V-tolerant**. The car is electrically
filthy (alternator noise, load dumps, inductive spikes).

- **Minimum viable:** resistor divider (e.g. 10k over 20k), 5V → 3.3V into UART RX. Polarity is
  already UART-shaped at logic level (idles high, pulses low = start bit) — no inversion needed.
- **More robust:** NPN transistor buffer or optocoupler (protection/isolation; both invert — set
  `UART_RXD_INV`). An opto gives galvanic isolation: the gold standard here.
- Small series resistor + clamp diode on the input is cheap insurance either way.

### Power — NOT from the ALDL connector

Unlike OBD-II (mandated always-hot 12V on pin 16), the 12-pin ALDL has **no standardized supply
pin**. Pin G on many 1227747-era vehicles is the *fuel pump prime/test lead* — it shows ~12V only
while the pump relay is energized, drops out ~2s after key-on-engine-off (exactly when you're
connecting), and shares the fuel pump's circuit. Do not power from it.

Options, in order of preference:

1. **12V accessory socket + USB buck adapter** — zero wiring, ignition-switched, trivial.
2. **Add-a-fuse tap** on an ignition-switched circuit + 12V→5V buck, for a permanent install.
3. USB power bank for development sessions.

Ground from pin A is fine and wanted regardless (buck ground and pin A are both chassis
potential).

Before trusting any pinout claim on the actual car: multimeter the connector at key off / key on /
engine running. Per-vehicle ALDL variation is the kind of thing the FSM gets right and internet
charts get wrong.

## Build & validation plan (ESP32 bridge)

Breadboard first, solder later. Acceptance test at every step is the existing capture statistics
(the 99.98%-clean `0xFE`/`0x00` check, PROM-ID match rate).

1. **Bench:** ESP32 UART listens to a PC replaying a committed `.raw` capture through a second
   USB-serial adapter — validates firmware + byte values with no car.
2. **At the car, wired:** ESP32 as a USB serial passthrough to the Mac; `goaldl record` + byte
   stats prove the analog input stage is clean.
3. **Networked:** add the WiFi-TCP (or Ethernet-TCP) transport; desktop `goaldl` with a
   `TCPProvider` validates end-to-end against a simultaneous or reference PL2303 capture.
4. Only then commit the circuit to protoboard/solder.
5. Mobile apps (gomobile-bound core + native UI + platform BLE) consume the same byte stream last.

Bill of materials: ALDL mating plug, 10k mode resistor, divider resistors (or transistor/opto +
bits), ESP32 dev board, buck converter or power bank. Weekend-project tier.

## App distribution notes

- **iOS:** personal use doesn't need the App Store — free Apple dev account sideloads with 7-day
  re-signing; $99/yr gives 1-year signing + TestFlight. MFi mode adds the accessory-protocol
  coordination noted above for Store distribution only.
- **Android:** sideload an APK freely; no program required.
