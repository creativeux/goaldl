<!-- SDA: v1.0 -->
# Plan: WinALDL Parity — Priority-Ordered Evolution

Priorities weigh user value (PM: diagnose > tune > log-UX polish) against cost (Architect: what the Session/Snapshot API and `blm.Grid` already give us). QA verification per phase relies on the committed real captures and the WinALDL ground-truth log.

## Architecture approach (applies to all phases)

- **Flag/code knowledge is ECM data, not code**: extend `ecm.Definition` with data tables — `FlagWords` (word name, byte offset, 8 bit labels) and `ErrorCodes` (code number, description, byte offset, bit) — populated for the 1227747 from A033.ads, cross-checked against the WinALDL screenshots. The parser stays generic.
- **Snapshot carries everything**: add parsed flags/codes (and later grid state stays consumer-side) to `stream.Snapshot` as plain serializable data — the future `serve` adapter inherits parity for free (success criterion 4).
- **Grids reuse `pkg/blm.Grid`**: INT and O2 are the same RPM×MAP accumulator with different value sources and gating (O2 ungated; INT closed-loop). Spark needs a small delta-counter variant (cumulative KNOCK_CNT byte → per-cell increments).
- **Bit-order verification (QA gate)**: MWAF1 bit numbering in `fueltrim.go` (bit7=closed loop, bit1=BLM enable) must reconcile with the WinALDL checkbox order; verify every flag/code bit against A033.ads + the ground-truth log before trusting labels.

## Phase 1 — Diagnose: see everything the ECM says *(proposed MVP)*

| Step | Delta | What | Notes |
|------|-------|------|-------|
| 1.1 | D1+D2 | Parse MW2, MALFFLG1-3, MWAF1, MCU2IO, KNOCK_CNT in `pkg/ecm` as data tables; expose flags+codes on `Snapshot` | Foundation for everything below; unit-test bits against ground-truth log |
| 1.2 | D1 | **Error codes tab**: set codes shown prominently (code + description), unset dimmed | Highest diagnostic value per line of code |
| 1.3 | D2 | **Flag data tab**: three grouped checklists (MW2/MWAF1/MCU2IO) | Explains *why* BLM isn't recording (loop state, idle, TCC…) |
| 1.4 | D3 | Sensor table: US+metric dual columns, TPS % (calibration flags, default 0.54/4.60V), MAP kPa, knock-count row | MAP kPa column doubles as the `MapVoltsToKPa` verification vs WinALDL (open backlog item) |
| 1.5 | D5 | **Raw history grid**: labeled per-byte rows × last N frames, decimal, scrolling | Replaces the single-frame hex view; subsumes the planned "live raw view" |
| 1.6 | D11+D16 | Footer heartbeat: per-frame good/bad tick (ParseOK/PROMOK), byte/frame count; views gate updates on ParseOK | Cheap; matches WinALDL's bad-sample gating |

**Exit criteria**: at the car, codes/flags/sensors/raw are all readable live; all new decodes covered by unit tests against the ground-truth log; existing golden tests untouched (no decoder changes).

## Phase 2 — Tune: full grid parity

| Step | Delta | What | Notes |
|------|-------|------|-------|
| 2.1 | D6 | **INT grid tab** (closed-loop gated, Wide Avg, same dimming/highlight) | `blm.Grid` reuse |
| 2.2 | D7 | **O2 grid tab** (ungated, volts, 3 decimals) | `blm.Grid` reuse |
| 2.3 | D9 | **Clear (`c`) / Save (`s`) keys** on grid tabs → timestamped file with Average + Correction tables (same format as `blm` command) | Parity with Clear/Save Table buttons |
| 2.4 | D4 | Sensor Latest/Max/Min modes (`m` cycle, `c` clear) | Small TUI-model state |

**Exit criteria**: a live tuning session needs no post-hoc `blm` run; saved tables match `blm` command output for the same capture (golden test).

## Phase 3 — Session UX

| Step | Delta | What |
|------|-------|------|
| 3.1 | D10 | Recording toggle (`r`) in TUI — live source writes raw capture on demand (needs switchable `SerialProvider` sink) |
| 3.2 | — | Replay pause/speed keys (space, +/-) — already on the project backlog |
| 3.3 | D8 | Spark-counts grid tab (knock-delta accumulator) |
| 3.4 | D10 | CSV logging toggle in TUI (reuse `csv.go` writer) |

## Phase 4 — Deferred / opportunistic
- D15 Dash view (big-number readout) — nice at the car, trivial once Snapshot has everything.
- D13 config-file persistence (`~/.goaldl`?) for port/ECM/TPS calibration.
- D14 additional ECM definitions — data-only work, driven by demand.
- Explicit non-goals stay out: Narrow/Avg10/StdDev modes, Windows-style config dialog, decode-path filtering.

## MVP decision — AGREED 2026-07-04

**MVP = Phase 1** (diagnose parity), confirmed by user. Rationale (PM): it converts the dashboard from "BLM tool with a sensor list" into the thing you'd actually bring to the car instead of WinALDL — codes, flags, full sensors, raw stream. Phase 2 is the tuning payoff but BLM (the primary tuning metric) already works live; INT/O2/save follow as a fast next iteration since `blm.Grid` reuse makes them cheap.

## Verification (QA)
- Unit tests: flag/code bit decode against hand-picked frames from `data/20250601_111156_LOG.txt` (WinALDL's own decode is the oracle).
- Replay-driven TUI tests (`tui_test.go` pattern) for each new tab against `drive_4800.raw`.
- Golden test: TUI grid save output vs `blm` command output on the same capture.
- No changes to `pkg/decoder` — its golden tests must stay byte-identical.
