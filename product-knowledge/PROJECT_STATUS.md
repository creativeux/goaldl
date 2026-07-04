<!-- SDA: v1.0 -->
# GLaDOS System Status

This document reflects the *current state* of the codebase and project.
It should be updated whenever a significant change occurs in the architecture, roadmap, or standards.

## Project Overview
**Mission**: [product-knowledge/MISSION.md](MISSION.md) — a working, cross-platform scanner/datalogger/tuning aid for GM 160-baud ALDL ECMs (primary target: GM 1227747), built in Go with an ordinary USB serial adapter.
**Current Phase**: Implementation — core decode pipeline validated on real hardware; building out the consumer/visualization layer (TUI dashboard, BLM tuning views, future `serve` adapter).

## Architecture

**Tech stack**: Go 1.26, single module `goaldl`. Direct deps: `go.bug.st/serial` (serial I/O), `charmbracelet/bubbletea` + `lipgloss` (TUI). No database, no network layer (yet).

**Pattern**: layered pipeline with a facade core API.

```
serial port / capture file
      │  (pkg/serial thin wrapper · pkg/stream Providers)
      ▼
pkg/decoder   byte-value state machine: UART bytes → bits → 20-byte frames
      ▼
pkg/ecm       frame layout knowledge (A033.ads): frame → Sensors, FuelTrim, PROM
      ▼
pkg/stream    Session facade → Snapshot stream (plain serializable data, no UI)
      ▼
consumers     cmd/goaldl TUI dashboard (default UX) · monitor/blm/decode
              scripting commands · pkg/blm grid accumulator · future serve adapter
```

Key boundaries:
- `pkg/decoder` is a faithful transport — no plausibility filtering; quality signals ride alongside as fields (see standards/decoder/raw-data-policy.md).
- `pkg/ecm` owns frame-layout knowledge; `pkg/blm` stays a generic RPM×MAP grid accumulator.
- `pkg/stream.Session`/`Snapshot` is the one API front-ends build on; terminal rendering (`SensorTable`, `BLMBody`) is presentation layered on top.
- `cmd/goaldl/main.go` dispatches on a first-arg command word (record/decode/monitor/blm/simulate/ports/ecms/help); everything else falls through to the TUI dashboard.

**Ground truth**: `data/20250601_111156_LOG.txt` (WinALDL log from the real vehicle) and real captures committed as test fixtures in `pkg/decoder/testdata/` with `.golden` frame dumps.

**Health**: `go test ./...` green (unit + golden + real-capture regression tests in decoder/ecm/blm/stream/cmd); gofmt-clean; CI (`.github/workflows/ci.yml`) gates gofmt · vet · build · `test -race` on push/PR. No linter beyond vet; no coverage tooling.

## Current Focus

### 1. Consumer / visualization layer
*Lead: Architect*
-   [x] TUI dashboard (Bubble Tea) as the default face, driven by `stream.Session`
-   [x] Core Session/Snapshot API refactor (branch `feat/tui-session-api`)
-   [ ] **Epic: WinALDL parity** (spec: `specs/2026-07-04_feature_winaldl-parity/`) — delta documented (D1–D16), 4-phase plan; **MVP = Phase 1 agreed 2026-07-04**
    -   [x] Feature: Phase 1 diagnose parity (MVP) — flags/codes/knock parsed as ecm data tables onto Snapshot; 5 TUI tabs (Sensors/BLM/Flags/Codes/Raw history); dual-unit sensor table (TPS% with -tps0/-tps100, MAP kPa); heartbeat + ParseOK gating. Implemented + verified 2026-07-04.
    -   [x] Feature: Phase 2 tune parity (INT/O2 grids, in-TUI save/clear, always-on Min/Max, persistent loop-status chrome) — **shipped + verified 2026-07-04** ([spec-phase2.md](../../specs/2026-07-04_feature_winaldl-parity/spec-phase2.md), [evaluation.md](../../specs/2026-07-04_feature_winaldl-parity/evaluation.md) PASS). Tabs regrouped to Sensors·BLM·INT·O2·Flags·Codes·Raw (keys 1–7); `s` saves all 3 grids, `c` context-clears; consumer-side accumulation only (no Snapshot/Session/blm/ecm change; decode path untouched, goldens byte-identical)
    -   [x] Feature: Phase 3 session UX (recording toggle `r` via fail-soft `RecordSink`, CSV toggle `d`, replay pause/speed keys, spark grid tab 5 on WinALDL axes, filename prompt on s/r/d with exclusive-create, self-expiring no-op warnings) — **shipped + verified 2026-07-04** ([spec-phase3.md](../../specs/2026-07-04_feature_winaldl-parity/spec-phase3.md), [evaluation.md](../../specs/2026-07-04_feature_winaldl-parity/evaluation.md) PASS: 10/10 criteria, forbidden seam diff empty, goldens byte-identical, BLM 469 preserved; both evaluator warnings fixed same-session)
-   [ ] `serve` adapter (HTTP/WebSocket) proving the Session API drives a non-terminal front-end

### 2. Backlog / Upcoming
-   [x] ~~Verify `MapVoltsToKPa` transfer~~ — **RESOLVED 2026-07-04**: WinALDL log proves kPa = (raw+28.06)/2.71; formula corrected (was ~3 kPa low), BLM cell values now match WinALDL's own table (117.17 vs 117.5 at 1600×40)
-   [ ] Coolant curve divergence (observation, accepted): WinALDL uses a smooth curve ~3°F below our stepped A033 table at warm idle; A033 table kept as authority
-   [ ] Optional phase 2: onboard MCU datalogging bridge (bot-thoughts method)

## Known Issues / Technical Debt
- VSS/vehicle speed reads 0 on this vehicle — byte 5 genuinely 0x00 (not wired or captured stationary); not a decoder issue.
- `DECODER_STATUS.md` / `HARDWARE_DECODING.md` (git history) predate the byte-value diagnosis — historical record only, not guidance.
- macOS PL2303 requires Prolific's App Store DriverKit app; counterfeit/pre-2012 chips are driver-blocked. Fallback: FTDI FT232R.
- No test files in `pkg/aldl` (shared Frame type only), `pkg/errors`, `pkg/serial` (thin hardware wrapper).

## Adoption Status
**Adopted**: 2026-07-04 (validated by user)
**Coverage**: full Go source tree (~3800 lines), CI config, README/CLAUDE.md docs, test suite
**Gaps**: `docs/winaldl/` reference PDFs not deeply analyzed; `data/*.ads` definition files taken as given

## Inferred Conventions
- Byte-value decoding, never host-side timing — Confidence: High (documented + enforced by history)
- Raw-data-raw policy (no filtering in decode path) — Confidence: High (documented, user-confirmed)
- Real captures as golden test fixtures, `-update` flag to regenerate — Confidence: High
- Session/Snapshot core API, UI as consumer — Confidence: High (recent deliberate refactor)
- Heavy doc-comments encoding physical-protocol reasoning at point of use — Confidence: High
- Standard Go tooling only (gofmt/vet/test -race), no extra linters — Confidence: Medium (may be by omission)

## Recent Changes
- 2026-07-04: **WinALDL parity Phase 3 (Session UX) verified & closed** — fresh-context evaluator returned PASS ([evaluation.md](../../specs/2026-07-04_feature_winaldl-parity/evaluation.md)): all 10 acceptance criteria + 5 standards + 2 core philosophies met; forbidden seam (`session.go`/`ecm`/`decoder`/`go.mod`) diff empty; goldens byte-identical; `blm` 469 preserved; pacing proven ±1 ms via injectable clock. Two warnings fixed same-session (stale "7 Raw" waiting message; stale CLAUDE.md rewritten for the 8-tab dashboard). Spec reconciled (RecordSink.Set byte count, promptState.hint, dir-"" verbatim paths); evaluator's untested-`closeOutputs` note closed with `TestTUICloseOutputs`. ROADMAP Phase 3 → ✅. Next: `serve` adapter.
- 2026-07-04: **WinALDL parity Phase 3 (Session UX) implemented** — in-TUI raw-recording toggle (`r`, via a new concurrency-safe `stream.RecordSink` behind the existing `SerialProvider.Sink` seam, fail-soft on write errors), CSV-logging toggle (`d`, `frameCSV` reuse, ParseOK rows), replay pause/speed keys (`space`/`+`/`-`, re-anchored non-retroactive pacing on `ReplayProvider`), spark-counts grid (tab 5, WinALDL axes, knock-delta accumulation with mod-256 wrap), and the filename prompt on all three file actions (exclusive create — no silent overwrite). `pkg/blm` gains only generic `Sum()`/`NewSpark()`; `Snapshot`/`Session`/`ecm`/decoder untouched (goldens byte-identical). Full suite green under `-race`; end-to-end fixture test cross-checks BLM==469 + independent spark recomputation. Awaiting verify-feature.
- 2026-07-04: **WinALDL parity Phase 3 (Session UX) spec'd** — `spec-phase3.md` + persona review (PM/Architect/QA approve) + pre-implementation standards gate (PROCEED). User decisions: filename prompt on all three file actions (s/r/d, fulfils the deferred editable-filename request), WinALDL spark axes (400–3600/400 × 30–100/5), Spark as tab 5 grouped with the grids (8 tabs). Key finding: Snapshot/Session/ecm unchanged again — recording (`stream.RecordSink` behind the existing `SerialProvider.Sink` seam, fail-soft) and replay pause/speed (mutex-guarded, re-anchored pacing) land on the providers; `pkg/blm` gains only generic `Sum()`/`NewSpark()`; no new dependencies.
- 2026-07-04: **WinALDL parity Phase 2 (Tune) verified & closed** — fresh-context evaluator returned PASS ([evaluation.md](../../specs/2026-07-04_feature_winaldl-parity/evaluation.md)): all 8 acceptance criteria + 5 standards + 2 core philosophies met, forbidden packages untouched, goldens byte-identical, `blm` still 469. One non-blocking note fixed (saved `goaldl_*.txt` grids added to `.gitignore`); spec reconciled (INTBody/O2Body value params, `writeTrimGridFile` name). ROADMAP + CLAUDE.md (7-tab dashboard) updated.
- 2026-07-04: **WinALDL parity Phase 2 (Tune) implemented** — INT + O2 grid tabs (blm.Grid reuse; INT closed-loop gated, O2 ungated 2-dec live/3-dec saved), always-on sensor MIN/MAX columns, in-TUI `s` save-all-grids / `c` context-clear, and a persistent loop-status line on every tab (green closed / amber open + per-grid ●/○ recording dots). Presentation + consumer-side accumulation only — no Snapshot/Session/blm/ecm change; decoder goldens byte-identical. New `pkg/stream/gridviews.go`; `tui.go` regrouped to 7 tabs. Full suite green under `-race`; end-to-end drive-fixture test cross-checks BLM==469 vs the blm command. Trace: `specs/2026-07-04_feature_winaldl-parity/`.
- 2026-07-04: **WinALDL parity Phase 2 (Tune) spec'd** — `spec-phase2.md` + persona review (PM/Architect/QA all approve) + pre-implementation standards gate (PROCEED). Key finding: Phase 2 is presentation + consumer-side accumulation only — no `Snapshot`/`Session`/`pkg/blm`/`pkg/ecm` change, decode path untouched.
- 2026-07-04: **WinALDL parity Phase 1 (MVP) shipped** — flags/error-codes/knock decoded as ecm data tables (A033.ads-verified bit order), Snapshot carries Flags/Codes, TUI grew to 5 tabs (Flags, Codes, scrolling Raw history) + heartbeat/gating, sensor table dual-unit (TPS%, MAP kPa). `MapVoltsToKPa` corrected to the WinALDL-verified transfer. Spec trace: `specs/2026-07-04_feature_winaldl-parity/`.
- 2026-07-04: Adopted into GLaDOS framework (SDA v1.0); standards/philosophies extracted from CLAUDE.md and code.
- 2026-07-03: Refactor to core Session API; TUI dashboard as default face; PR review fixes (branch `feat/tui-session-api`).
- 2026-07-03: Consolidation — 23 experimental tools and 6 dead decoders deleted (7214 → ~1775 lines then), repo under git, test suite rooted in real captures.
- 2026-07-03: **Hardware validation** — 635/635 frames PROM-matched across a 14-minute real drive capture.
