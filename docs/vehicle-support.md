# Vehicle support matrix

goaldl today decodes **one** ECM: the GM **1227747** (160-baud, ALDL datastream
`A033`). This document maps the *addressable* range — the full population of
GM ALDL vehicles from roughly **1980–1995** (pre-OBD-II) — so we can prioritise
what to support next and see what each addition actually costs in the engine.

> **Scope & confidence.** The protocol/baud/connector facts below are
> well-corroborated (Tech Edge, Wikipedia, and multiple tuning communities
> agree). The **ECM-part-number → vehicle/year/engine** rows are drawn from
> community catalogs (TunerCat, moates.net, thirdgen, pcmhacking) and vary by
> calibration; treat any specific row as **needs-verification** until we have a
> real capture or a definition file in hand. Sources are listed at the bottom.

## The two populations

Every GM ALDL car falls into one of two serial speeds. This is the primary axis
for us, because the two need *different transport decoding*:

| | **160 baud** (what we support) | **8192 baud** |
|---|---|---|
| Era | Early — ~1980 to early 1990s | Later — mid/late 1980s to 1995 |
| Direction | Unidirectional **broadcast** (ECM talks, we listen) | **Bidirectional**, request/response (we must send a request) |
| Encoding | Pulse-width (the byte-value trick in [protocol.md](protocol.md)) | Conventional 8192-baud UART framing |
| Throughput | ~20 bytes/sec | up to ~1024 bytes/sec |
| 12-pin data pin | **Pin E** | **Pin M** |
| goaldl support | ✅ 1227747; other ECMs are new *definitions* only | ❌ needs a new transport + request layer |

> **Baud is not decided by fuel system.** A common myth ("carbureted = 160,
> EFI = 8192") is false — baud tracks ECM generation, not carb-vs-injection.
> Verification killed that claim outright.

### Connector

The classic **12-pin ALDL connector** (black, under the dash) is near-universal
for US GM 1982–1995. Key pins:

- **A** — vehicle ground
- **B** — diagnostic / mode-select. Jumper A↔B (ignition on, engine off) to
  flash 2-digit codes on the CHECK ENGINE lamp. Some late-'80s ECMs need a
  **10 kΩ** resistor A→B before the datastream starts.
- **E** — serial data on **160-baud** cars
- **M** — serial data on **8192-baud** cars

Earlier and export cars differ: a **5-pin** connector on some early cars, a
**10-pin** on Opel/Lotus, a **6-pin** on Holden VN/VP Commodore (pin A ground,
B test, H +12 V), and a **16-pin OBD-II-style** shell on the last OBD-1.5 cars
(VR/VS Commodore). The **1995** model year physically changed the US connector
(tools of that era shipped separate "CABL1" 86–94 and "CABL2" 95 cables).

## Support tiers

### Tier 0 — Supported today

| ECM P/N | Datastream | Mask | Baud | Vehicles (needs-verification) |
|---|---|---|---|---|
| **1227747** | `A033` | `$42` | 160 | ~1986–1993 GM TBI trucks/vans, 4.3 / 5.0 / 5.7 L, non-electronic trans; ran into the mid-'90s in manual-trans trucks |

Note: `A033` is the **ALDL datastream** definition (the frame layout goaldl
reads); `$42` is the **PROM/calibration mask** for the same ECM. They're
different axes — one ECM part number can carry several masks/calibrations.

### Tier 1 — Near-term: other **160-baud** ECMs

These are the cheapest wins — same transport we already have; each is "just" a
new ECM **definition** (`pkg/ecm`) describing a different frame layout. **No
decoder-core change.**

| ECM P/N | Baud | Vehicles / engines (needs-verification) |
|---|---|---|
| 1228747 | 160 | Grouped with 1227747 as 86–93 TBI |
| 1227165 | 160 | 1986–1989 TPI (Corvette / F-body 5.0–5.7 L) |
| 870 | 160 | 1985 TPI cars |
| 1227748 (`A097`) | 160 (switchable 8192) | 1990 Olds Cutlass Ciera 2.5 L Iron Duke TBI |
| 1227808 / P4 | ~160 | Holden VN Commodore 3.8 L (also LD Astra, JE Camira, Nissan Pulsar) — 12 V level, ~159 baud, two-byte PROM ID like the 1227747 |

### Tier 2 — Larger: **8192-baud** ECMs

These need a **new transport** (bidirectional request/response at 8192 baud) on
top of new definitions — a bigger lift than Tier 1.

| ECM P/N | Baud | Vehicles / engines (needs-verification) |
|---|---|---|
| 1226870 | 8192 | 1985 Corvette 5.7 TPI (L98), F-body 5.0 TPI (LB9), 2.8 V6 |
| 1227137 (`$27`) | 8192 | 1986 Astro/Safari, Caprice, LeSabre, Monte Carlo 4.3 (LB4) |
| 1227148 | 8192 | 1986–1987 Buick Turbo (Grand National) |
| 1227730 | 8192 | 1990–1992 TPI |
| 1227749 | 8192 | 1991–1993 GMC Syclone / Typhoon |
| 16159278 | 8192 | 1992–1993 Corvette 5.7 LT1 |

### Tier 3 — Transitional **OBD-1.5** (1994–1995)

Late ALDL, often reflashable over the port, sometimes an OBD-II-shaped
connector but still ALDL protocol. Boundary cases worth a dedicated look.

| ECM/PCM P/N | Def | Vehicles / engines (needs-verification) |
|---|---|---|
| 16181333 / 16188051 | `$EE` | 1994–1995 LT1 cars (reflashable over ALDL) |
| 16197427 | `A217`/`A218` | 1995 GMC Sonoma 4.3 (split engine/trans datastreams) |
| 16183247 | `A221` | 1995 Buick LeSabre H-body 3800 Series I |

### Out of current scope

- **OBD-II (1996+)** — a different protocol entirely; not ALDL.
- Non-GM pre-OBD-II serial links (Ford MCU/EEC, Chrysler SCI) — not ALDL.

## What each tier costs us

- **Tier 1 (160-baud ECMs):** add a `pkg/ecm` definition (parameters, flag
  words, byte labels) per ECM. The decoder, `Session`, and TUI are unchanged —
  they're already data-driven. Ground-truth capture per ECM strongly preferred.
- **Tier 2 (8192-baud):** a new provider/transport that *requests* frames and
  frames conventional 8192-baud UART bytes, plus a mode-select (10 kΩ / request)
  step, plus per-ECM definitions. The `Snapshot`/view layer can likely stay.
- **Tier 3 (OBD-1.5):** as Tier 2, plus connector/cable variance and split
  engine/trans datastreams.

## Sources

- ALDL 160-baud protocol & timing — Tech Edge: <https://www.techedge.com.au/vehicle/aldl160/160serial.htm>
- ALDL 8192 hardware & pinout — Tech Edge: <https://www.techedge.com.au/vehicle/aldl8192/8192hw.htm>
- Holden VN 160-baud datastream — Tech Edge: <https://www.techedge.com.au/vehicle/aldl160/vn_aldl.htm>
- ALDL connector overview — Wikipedia: <https://en.wikipedia.org/wiki/ALDL>
- ECM part-number / mask catalog — TunerCat: <http://www.tunercat.com/tnr_desc/ecm_sup.html>
- 1227747 TBI trucks — moates.net: <https://support.moates.net/tbi-trucks-1227747/>
- ECM-to-vehicle discussion — thirdgen.org, gmt400.com, tunerpro.net forums (see thread list in the research run)

_This matrix is direction-setting, not a compatibility guarantee. Rows marked
needs-verification should be confirmed against a real capture or definition file
before we claim support._
