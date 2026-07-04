<!-- SDA: v1.0 -->
# Requirements: WinALDL Parity — Functional Delta

**Goal**: Evolve the goaldl TUI dashboard from its current three-view state to relative parity with WinALDL 1.10a (the product it replaces), for the GM 1227747, prioritized by tuning/diagnostic value.

**Who**: The owner-tuner of the target vehicle (GM 1227747 TBI). Primary jobs: (1) diagnose — read error codes and ECM operating state at the car; (2) tune — collect BLM/INT/O2 data across the RPM×MAP envelope and derive corrections; (3) log — capture sessions for offline analysis.

**Sources**: `docs/winaldl/` — screenshots PDF (functionality description), 10 per-view GIFs, version history PDF (reveals heartbeat indicator, Dash dialog, bad-sample gating, active-cell highlight, hot keys), supported-ECMs PDF. Ground truth log: `data/20250601_111156_LOG.txt`.

---

## WinALDL feature inventory (observed)

**Chrome (always visible)**: Datalogger button, Configuration button, EXIT, byte-count "heartbeat" that flashes green (good sample) / red (missing bytes) per frame.

1. **RAW Data tab** — scrolling history grid: one labeled row per frame byte (MW2, PROMIDA/B, IAC, CT, MPH, MAP, RPM, TPS, INT, O2, MALFFLG1-3, MWAF1, VOLT, MCU2IO, KNOCK_CNT, BLM, O2_CNT), columns = latest ~14 samples (0, -1, … -13), decimal values, scrolls right as data arrives.
2. **Flag Data tab** — bit decode of three status words as labeled checkboxes: **MW2** (VSS pulse, ESC 43B 2nd power-enrich ready, DRP, diagnostic mode 0Ω, factory test 3.9kΩ, ALDL mode 10kΩ, Idle flag, 1st-time-idle), **MWAF1** (clear flood, BLM enable, low battery/IAC inhibited, async fuel, 4-3 downshift TCC unlock, old high gear, closed loop, rich), **MCU2IO** (AIR divert ×2, A/C disabled, TCC locked, park/neutral, no high gear, not used, no A/C requested).
3. **Sensor Data tab** — per sensor: RAW, converted US value+unit, converted metric value+unit (CT °F/°C, MPH/KPH, MAP V/kPa, TPS V/%). Rows: IAC, CT, MPH, MAP, RPM, TPS, INT, O2, battery, knock counter, BLM, rich/lean counter. Spin selector: **Latest / Max / Min** values + clear.
4. **Error codes tab** — MALFFLG1-3 decoded to trouble codes with descriptions, checkbox = set. MALFFLG1: 24 VSS, 23 IAT/MAT low, 22 TPS low, 15 CT low, 21 TPS high, 14 CT high, 12 engine not running, 13 O2 sensor. MALFFLG2: 42 EST, 41/35 not used, 33 MAP high, 34 MAP low, 32 EGR, 25 IAT/MAT high, 31 governor fail. MALFFLG3: 52 CAL pack missing, 53 not used, 54 fuel pump relay failure, 51 PROM error, 55 ADU error, 45 O2 rich, 43 knock ESC, 44 O2 lean.
5. **BLM tab** — RPM×MAP grid (400–3200 × 20–100 kPa), live cell highlight, populates in closed loop. Modes: Narrow/Wide × Latest/Avg/#Samples/Avg10/StdDev10. **Clear Table** / **Save Table** buttons (saved file includes correction tables).
6. **INT tab** — same grid for the integrator (short-term trim); populates in closed loop.
7. **O2 tab** — same grid for O2 voltage (3 decimals); populates immediately (no gating).
8. **Spark Counts tab** — RPM×MAP grid (400–3600 × 30–100 step 5); cell increments by actual knocks detected (delta of the cumulative knock counter).
9. **Datalogger dialog** — choose logs (RAW / Flag / Sensor / Error codes), US/Metric, START; logs limited only by disk.
10. **Configuration dialog** — ECM type, COM port, **TPS 0%/100% calibration voltages** (default 0.54/4.60 → TPS %), Narrow RPM/MAP ranges; persisted across restarts.
11. **Misc** — hot keys for display switching; "Dash" dialog (few variables in large digits); bad-sample gating (wrong byte count → only RAW tab updates, other tabs protected from garbage); multi-ECM support (many GM families).

## goaldl today

**TUI** (`cmd/goaldl/tui.go`): 3 tabs — Sensors (raw + one converted value, 12 params), BLM grid (Wide Avg live, min-samples confidence dimming, active-cell highlight, closed/open-loop status line), Raw (latest frame hex only). Footer: frame index, elapsed, PROM ✓/✗. Keys 1-3/tab/q. Live (`-p`) or replay (`-speed`).
**Scripting**: record, monitor (+`-csv`, +`-blm`), decode→CSV, blm→Average+Correction tables, simulate, ports, ecms.
**Engine**: `stream.Session`→`Snapshot` (frame + Sensors map + FuelTrim + PROMOK/ParseOK). `pkg/ecm` parses 12 params — **does not parse** MW2, MALFFLG1-3, MWAF1 (beyond 2 fuel-trim bits), MCU2IO, KNOCK_CNT. `pkg/blm.Grid` is a generic RPM×MAP accumulator (samples/average/correction).

## Functional delta (WinALDL ✓ / goaldl ✗)

| # | Capability | Gap |
|---|-----------|-----|
| D1 | Error codes view (MALFFLG1-3 → coded list) | Entirely missing; bytes not even parsed |
| D2 | Flag data view (MW2/MWAF1/MCU2IO bit decode) | Missing; only 2 MWAF1 bits decoded internally |
| D3 | Sensor table: dual US+metric, TPS %, MAP kPa, knock-count row | Single value column; TPS only volts; MAP only volts; no knock row |
| D4 | Sensor Max/Min capture modes + clear | Missing |
| D5 | RAW view: labeled per-byte scrolling history grid | Only latest frame as unlabeled hex |
| D6 | INT grid tab | Missing (Grid is reusable) |
| D7 | O2 grid tab (ungated, 3 decimals) | Missing (Grid is reusable) |
| D8 | Spark counts grid (knock deltas) | Missing |
| D9 | Save/Clear tables from the UI | Only offline via `blm` command; no INT/O2 save at all |
| D10 | In-UI datalogging toggle (raw and/or CSV) | Only pre-declared via CLI flags on record/monitor |
| D11 | Heartbeat / per-frame data-quality indicator | PROM ✓/✗ exists; no good/bad flash, no byte count |
| D12 | TPS calibration (0%/100% voltages) | Missing (needed for TPS %) |
| D13 | Config persistence across runs | CLI flags only (acceptable CLI idiom; revisit) |
| D14 | Multi-ECM definitions | Registry exists but holds one ECM |
| D15 | Dash (big-number) view | Missing |
| D16 | Bad-sample gating in views | Snapshot has ParseOK/PROMOK; views don't gate on it |

## Explicit non-goals (agreed project standards)
- **Narrow modes and Avg10/StdDev10 grid variants** — intentionally not built; Wide Average is the tuning metric (CLAUDE.md / standards).
- **Multi-ECM expansion** (D14) — mission targets the 1227747; the definition table stays data-driven so others can be added later.
- **Windows-dialog config UX** (D13) — flags + (optionally) a config file are the native CLI idiom.
- Plausibility filtering anywhere in the decode path (raw-data-raw policy); view-level gating (D16) is a *consumer* decision and is in scope.

## Success criteria
1. At the car, the TUI alone answers: any codes set? what mode/loop state is the ECM in? what is every sensor reading (US or metric)? — without WinALDL.
2. Tuning session parity: BLM + INT + O2 grids accumulate live with confidence dimming; tables can be cleared/saved from the TUI with correction output matching the `blm` command.
3. Every new decode (flags, codes, knock, conversions) is verified against the WinALDL ground-truth log (`data/20250601_111156_LOG.txt`) and/or the committed real captures, with golden/unit tests.
4. All new state flows through `stream.Snapshot` (plain data, serializable) so the future `serve` adapter gets flags/codes/grids for free.
