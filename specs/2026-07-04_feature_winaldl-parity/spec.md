<!-- SDA: v1.0 -->
# Spec: WinALDL Parity — Phase 1 (MVP, agreed 2026-07-04)

Scope: plan.md Phase 1 steps 1.1–1.6. Authorities: **A033.ads** for frame layout and bit meanings (its byte numbers are 1-based; our offsets are 0-based, ADS byte N → offset N−1); **`data/20250601_111156_LOG.txt`** (WinALDL's own decode of this vehicle) as the conversion/bit oracle.

## 0. Verified ground truth (research results, 2026-07-04)

- **Flag bit order = plain LSB bit numbering**, confirmed three ways: A033.ads `btBitNumber`, WinALDL log column order (bit 0→7 per word), and live data (row with MWAF1=64 sets exactly the bit-6 "Rich Flag" column; MW2=128 → bit-7 Idle; MCU2IO=128 → bit-7 No-A/C-request).
- **Existing `fueltrim.go` constants are correct** (MWAF1 bit 1 = BLM enable, bit 7 = closed loop). The plan's QA gate is resolved — no change to hardware-validated behavior.
- **MAP transfer verified**: WinALDL kPa = **(raw + 28.06) / 2.71** — exact across the log's raw range 49–190 (49→28.4, 101→47.6, 183→77.9). Current `MapVoltsToKPa` (21.25·V − 1.25) reads ~3 kPa low at idle. **Fix to the verified formula** (linear in raw: factor 1/2.71, bias 28.06/2.71 ≈ 10.354; in volts: 18.8256·V + 10.354). Closes the standing backlog item.
- **TPS % verified**: (V − V₀)/(V₁₀₀ − V₀) × 100 with WinALDL defaults V₀=0.54, V₁₀₀=4.60 (raw 28→0.2%, 39→5.5%, 62→16.6%).
- **Coolant divergence (accepted)**: WinALDL uses a smooth curve (raw 190→74.1 °F); our A033 stepped table gives 77 °F. Δ≈3 °F. Keep the A033 table (committed, tested); log as observation, not a defect.
- WinALDL/ADS label conflicts resolved in ADS's favor except where ADS is silent or the log's live data disproves it (noted per-bit below).

## 1. Data model — `pkg/ecm` (step 1.1)

New types (all plain data; parser stays generic):

```go
type FlagBit struct { Bit int; Name string; SetLabel string } // SetLabel e.g. "CLOSED" (optional)
type FlagWord struct { Name string; Offset int; Bits []FlagBit }
type ErrorCode struct { Code int; Description string; Offset int; Bit int }
```

`Definition` gains: `FlagWords []FlagWord`, `ErrorCodes []ErrorCode`, `ByteLabels []string` (len = FrameSize; WinALDL names: MW2, PROMIDA, PROMIDB, IAC, CT, MPH, MAP, RPM, TPS, INT, O2, MALFFLG1, MALFFLG2, MALFFLG3, MWAF1, VOLT, MCU2IO, KNOCK_CNT, BLM, O2_CNT).

Decode helpers returning plain serializable data:

```go
type FlagBitStatus struct { Name string; Set bool; SetLabel string }
type FlagWordStatus struct { Word string; Raw byte; Bits []FlagBitStatus } // all 8 bits, defined ones only
type CodeStatus struct { Code int; Description string; Set bool }
func DecodeFlags(def *Definition, frame []byte) []FlagWordStatus
func DecodeCodes(def *Definition, frame []byte) []CodeStatus // sorted by Code
```

### 1227747 flag tables (per A033.ads)

**MW2 (offset 0)**: 0 VSS pulse occurred · 1 Code-43 ready for 2nd test · 2 Reference pulse occurred · 3 Diag switch: factory test (3.9 kΩ) · 4 Diag switch: field test (shorted) · 5 Diag switch: ALDL (10 kΩ) · 6 Battery voltage high · 7 Idle flag. *(GIF checkbox order swapped 3/4; ADS + log column order win.)*

**MWAF1 (offset 14)**: 0 Cranked in clear flood · 1 BLM enable · 2 Battery voltage low (IAC inhibited) · 3 4-3 downshift for TCC unlock · 4 Async fuel · 5 High gear last pass · 6 Rich (SetLabel RICH) · 7 Loop status (SetLabel CLOSED).

**MCU2IO (offset 16)**: 0 AIR switch ON · 1 AIR divert ON · 2 A/C disabled *(WinALDL; absent from ADS)* · 3 TCC locked · 4 Park/Neutral · 5 High gear · 6 (not used — omit) · 7 No A/C requested *(live data: set at idle, A/C off — WinALDL's negated label matches reality)*.

### 1227747 error codes

**MALFFLG1 (offset 11)**: bit0 → **24** VSS · bit1 → **23** IAT/MAT low (unused on A033) · bit2 → **22** TPS low · bit3 → **21** TPS high · bit4 → **15** Coolant temp low · bit5 → **14** Coolant temp high · bit6 → **13** O2 sensor open · bit7 → **12** No reference pulses (engine not running).
**MALFFLG2 (offset 12)**: bit0 → **42** EST monitor · bit1 → **41** (not used) · bit2 → **35** IAC · bit3 → **34** MAP low · bit4 → **33** MAP high · bit5 → **32** EGR · bit6 → **31** Governor fail (unused on A033) · bit7 → **25** IAT/MAT high (unused on A033).
**MALFFLG3 (offset 13)**: bit0 → **55** A/D unit · bit1 → **54** Fuel pump relay · bit2 → **53** (not used) · bit3 → **52** CAL-PACK missing · bit4 → **51** PROM error · bit5 → **45** O2 rich · bit6 → **44** O2 lean · bit7 → **43** ESC (knock).

### Parameter additions (sensor values)

- `knock_count` (offset 17, counts, factor 1) — new row.
- **Dual-unit**: `Parameter` gains `Alt *AltConversion{Factor, Bias float64; Lookup Lookup; Unit string}` — same shape as the primary conversion, purely data. 1227747 Alt set:
  - CT: Lookup = °C via existing °F table, (F−32)/1.8 — unit °C
  - MPH: factor 1.609 — unit KPH
  - MAP: factor 0.369 (=1/2.71), bias 10.354 — unit kPa *(the verified transfer, applied to raw)*
  - TPS: **percent** — filled at startup from calibration (see §3); default (raw·0.0196 − 0.54)/(4.60−0.54)·100
- **`MapVoltsToKPa` in fueltrim.go changes** to the verified formula (18.8256·V + 10.354). Consequence: BLM cell binning shifts slightly (idle ~78 kPa vs ~75). `pkg/blm`/`pkg/stream` test expectations that encode old kPa values must be updated — intended change, WinALDL-verified. Decoder goldens are unaffected (no decode-path change).

## 2. API — `stream.Snapshot` (step 1.1)

```go
type Snapshot struct {
    FrameEvent
    PROMOK, ParseOK bool
    Sensors  map[string]float64
    FuelTrim ecm.FuelTrim
    Flags    []ecm.FlagWordStatus // nil when frame too short
    Codes    []ecm.CodeStatus     // nil when frame too short
}
```

Populated in `Session.snapshot()` via `DecodeFlags`/`DecodeCodes`. Everything stays plain data → `serve` adapter inherits parity (success criterion 4).

## 3. TUI (steps 1.2–1.6) — `cmd/goaldl/tui.go`

**Tabs**: `1 Sensors · 2 BLM grid · 3 Flags · 4 Codes · 5 Raw` (existing 1/2 muscle memory preserved; tab/arrows cycle all 5).

- **Codes view (1.2)**: three groups (MALFFLG1-3). Set codes rendered bold/highlighted at top of group ("Code 44 — O2 lean"), unset dimmed. Codes marked "not used" only shown if set (unexpected → worth seeing).
- **Flags view (1.3)**: three columns/groups (MW2, MWAF1, MCU2IO), `[x]`/`[ ]` checklists, SetLabel appended when set (e.g. "Loop status: CLOSED").
- **Sensor view (1.4)**: columns SENSOR · RAW · VALUE · ALT (`BuildRows` gains the Alt column; "—" when no Alt). New flags on `cmdTUI` (TUI + monitor share them): `-tps0` (default 0.54) and `-tps100` (default 4.60); guard v₁₀₀>v₀ else fall back to defaults with a stderr note. Definition's TPS Alt is filled from these at startup (definition copy, registry untouched).
- **Raw view (1.5)**: labeled scrolling history — rows = 20 bytes with `ByteLabels`, columns = latest N frames (newest left, header `0 -1 -2 …`), decimal values right-aligned. N = fit to terminal width (min 1, cap 14 like WinALDL). Ring buffer of raw frames in the TUI model (bounded, e.g. 64). Keep byte offset + PROM mark line.
- **Heartbeat + gating (1.6)**: footer becomes `frame N t=…s PROM ✓ ● 635 ok / 0 bad`; ● rendered green when the latest snapshot ParseOK && PROMOK, red otherwise. **Gating**: raw view always updates; sensor/flags/codes/BLM views render from `lastGood` (latest snapshot with ParseOK; PROM mismatch does NOT gate — raw-data-raw, PROM is a quality signal not a filter). BLM accumulation already self-gates via `FuelTrim.Recordable()`.

## 4. Edge cases

- Frame shorter than a word/code offset → `DecodeFlags`/`DecodeCodes` return nil; views show "waiting for frames…" state as today.
- ParseOK never true (e.g. garbage capture) → sensor/flags/codes show waiting state; raw view still streams (WinALDL behavior).
- Terminal narrower than one raw history column → clamp to 1 column.
- TPS calibration degenerate (v₁₀₀ ≤ v₀) → warn, use defaults.
- Replay ends → existing "(stream ended)" retained; last-good views stay rendered.

## 5. Test plan (QA)

| Test | Where | Oracle |
|---|---|---|
| Flag decode: MWAF1=64→only Rich; MW2=128→only Idle; MCU2IO=128→only No-A/C-req; MWAF1=0→none | `pkg/ecm` | log rows (verified above) |
| Code decode: each MALFFLG byte with a single bit → exactly that code; 0→none set; multi-bit | `pkg/ecm` | A033.ads table |
| FuelTrimSample ↔ flag-table consistency (bit1/bit7 same source of truth) | `pkg/ecm` | cross-check |
| MapVoltsToKPa: raw 49→28.4, 101→47.6, 183→77.9 (±0.05 kPa) | `pkg/ecm` | WinALDL log |
| TPS %: raw 28→0.2, 39→5.5, 62→16.6 (±0.05) | `pkg/ecm` or table test | WinALDL log |
| Alt conversions: CT °C, MPH→KPH | `pkg/stream` BuildRows | arithmetic + sensordata.gif (177.1°F/80.6°C) |
| Snapshot carries Flags/Codes over drive fixture | `pkg/stream` session test | drive_4800.raw |
| TUI: 5 tabs render; raw history scrolls & clamps; bad-frame gating holds lastGood; footer counts | `cmd/goaldl/tui_test.go` | replay-driven |
| Regression: decoder goldens byte-identical; blm tests updated only for the intended kPa shift | existing suites | — |

## 6. Out of scope (Phase 1)

INT/O2/spark grids, save/clear keys, Max/Min sensor modes, recording/CSV toggles, replay pause keys, config persistence, Dash view — Phases 2–4 per plan.md.
