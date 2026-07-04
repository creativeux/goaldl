<!-- SDA: v1.0 -->
# Trace: WinALDL Parity (TUI Dashboard)

**Workflow**: plan-feature
**Started**: 2026-07-04
**Feature**: winaldl-parity — document the functional delta between the goaldl TUI and WinALDL, and plan the priority-ordered evolution to relative parity, with an agreed MVP stopping point to implement now.

## Active Personas
- Product Manager — parity prioritization, MVP cut line
- Architect — mapping gaps onto Session/Snapshot API, cost/sequencing
- QA — per-view verification criteria (fixtures, golden tests, replay-driven TUI tests)

## Active Capabilities
- Read (PDF/image analysis) — analyzing WinALDL screenshots PDF and per-view GIFs in docs/winaldl/
- Bash/Go toolchain — build/test/replay against committed real captures (pkg/decoder/testdata/)
- Replay-driven TUI verification — dashboard runs headless-testable via stream.Session + drive_4800.raw fixture
- Subagents — available for context-isolated evaluation in later verify workflows

## Log
- 2026-07-04: Session start. Feature name confirmed as `winaldl-parity`; personas PM + Architect + QA selected by user.
- 2026-07-04: Reference material inventory: 3 PDFs (screenshots, supported ECMs, version history) + 10 view GIFs (sensordata, rawdata, flagdata, errorcodes, blm, int, o2, spark, log, config).
- 2026-07-04: Analyzed all 10 WinALDL views (GIFs), functionality text (screenshots PDF), and version history (surfaced non-obvious features: heartbeat indicator, bad-sample gating, Dash dialog, TPS calibration). Compared against cmd/goaldl/tui.go, pkg/ecm, pkg/stream, pkg/blm.
- 2026-07-04: Wrote [requirements.md](requirements.md) — 16-item functional delta (D1–D16) + explicit non-goals (Narrow/Avg10/StdDev modes, multi-ECM expansion, dialog-style config) + success criteria.
- 2026-07-04: Wrote [plan.md](plan.md) — 4 phases: (1) Diagnose parity (codes/flags/sensor enrichment/raw history/heartbeat), (2) Tune parity (INT+O2 grids, save/clear, max/min), (3) Session UX (record toggle, replay keys, spark grid), (4) Deferred. MVP recommendation: Phase 1; alternative cut Phase 1+2. Pending user decision.
- 2026-07-04: Key technical decisions logged in plan: flag/code knowledge as ecm.Definition data tables (parser stays generic); everything exposed via stream.Snapshot (serve-adapter-ready); INT/O2 reuse blm.Grid; QA gate on MWAF1 bit-order reconciliation vs A033.ads before trusting labels.
- 2026-07-04: **MVP cut agreed with user: Phase 1** (diagnose parity — steps 1.1–1.6). Phases 2–4 remain planned backlog. Proceeding to spec-feature → implement-feature.
- 2026-07-04: spec-feature: researched A033.ads BITS section (authoritative bit map; ADS byte numbers are 1-based) and cross-verified against WinALDL log columns AND live row data (MWAF1=64→Rich, MW2=128→Idle, MCU2IO=128→No-A/C-req). **QA gate resolved**: fueltrim.go bit constants confirmed correct. **Two verified corrections discovered**: MAP kPa = (raw+28.06)/2.71 (current MapVoltsToKPa ~3 kPa low — closes standing backlog item), TPS% = (V−0.54)/(4.60−0.54)×100. Coolant table diverges ~3°F from WinALDL's smooth curve — accepted, logged as observation. Wrote [spec.md](spec.md).

## Persona Review (spec.md)
- **Product Manager**: Scope matches the agreed Phase 1 cut exactly; out-of-scope list explicit; success criteria testable per view. Requested set codes render prominently vs dimmed unset — spec §3 covers it. **Approve.**
- **Architect**: Conversion-stays-data preserved (Alt on Parameter mirrors Factor/Bias/Lookup); parser generic; flags/codes are ecm-layer knowledge exposed as plain data on Snapshot (serve-ready); no new dependencies. Duplication risk between fueltrim.go constants and the MWAF1 flag table is mitigated by a consistency test rather than a refactor — acceptable, hardware-validated code untouched. **Approve.**
- **QA**: Every test row has a named oracle (log row / ADS / fixture); edge cases defined (short frame, never-ParseOK, narrow terminal, degenerate TPS calibration); regression strategy explicit. Added requirement: after the MapVoltsToKPa fix, rerun `blm` over the drive fixture and sanity-compare against data/20250601_162123_BLM.txt cell placement. **Approve.**

## Standards Gate Report (pre-implementation)
| Standard | Scope | Severity | Verdict |
|---|---|---|---|
| decoder/byte-value-decoding | decoder | must | ✅ PASSES (decode path untouched) |
| decoder/raw-data-policy | decoder | must | ✅ PASSES (gating is view-level; PROM mismatch never filtered) |
| testing/golden-fixtures | testing | should | ✅ PASSES (decoder goldens byte-identical; blm expectation updates are the intended, documented kPa change) |
| architecture/session-api-layering | architecture | must | ✅ PASSES (flags/codes in pkg/ecm, exposed via Snapshot; TUI consumes) |
| go/tooling | go | should | ✅ PASSES (no new deps; gofmt/vet/race gate) |
| philosophy: consolidate-over-accrete | core | — | ✅ (raw view replaced, not duplicated; growth as Session consumer) |
| philosophy: ground-truth-first | core | — | ✅ (every conversion/bit anchored to log or ADS; MapVoltsToKPa assumption resolved with evidence) |

Gate decision: **PROCEED to implement-feature.**

## Implementation (2026-07-04)
Files changed (see [tasks.md](tasks.md) for the step mapping):
- `pkg/ecm/flags.go` (new) — FlagBit/FlagWord/ErrorCode data types + generic DecodeFlags/DecodeCodes returning plain-data statuses.
- `pkg/ecm/ecm.go` — AltConversion + Parameter.Alt; Definition gains FlagWords/ErrorCodes/ByteLabels; definition-level `Parse` (Registry.ParseFrame delegates); TPSPercentAlt + WithTPSCalibration (copy-on-write, degenerate-range no-op).
- `pkg/ecm/gm_1227747.go` — knock_count parameter; Alt conversions (CT °C lookup, MPH→KPH, MAP kPa, TPS % default cal); MW2/MWAF1/MCU2IO flag tables; 24 MALFFLG trouble codes; 20 byte labels.
- `pkg/ecm/fueltrim.go` — MapVoltsToKPa corrected to the WinALDL-verified (raw+28.06)/2.71.
- `pkg/stream/session.go` — Snapshot gains Flags/Codes; Session caches the definition.
- `pkg/stream/table.go` — 4-column table (ALT); Renderer/SensorTable take a Definition (calibration-aware); formatting shared via formatNum.
- `pkg/stream/statusviews.go` (new) — FlagsBody/CodesBody/RawHistory pure content builders (inline ANSI emphasis, same idiom as BLMBody).
- `cmd/goaldl/tui.go` — 5 tabs (Sensors/BLM/Flags/Codes/Raw), raw-history ring (64 frames, ≤14 columns), heartbeat footer with ok/bad counts, ParseOK gating via lastGood, -tps0/-tps100 flags.
- `cmd/goaldl/monitor.go` — -tps0/-tps100, renderer over the calibrated definition.
- Tests: `pkg/ecm/flags_test.go` (new, log-oracle), `pkg/stream/statusviews_test.go` (new), session/stream/tui/fueltrim/blm tests extended or re-derived.

Test results: `go vet` clean, `go test -race ./...` all green, gofmt clean. Decoder goldens byte-identical (decode path untouched). `TestAccumulateBLM` expectations re-derived for the corrected MAP transfer — sanity-confirmed against WinALDL's own BLM table for this vehicle (our 1600 RPM×40 kPa average 117.17 vs WinALDL's 117.5 from a different drive session; the old transfer put 116.0 in that cell by sampling a shifted pressure band). End-to-end: `monitor` over the idle fixture renders the ALT column (104 °F/40 °C, 37.66 kPa, TPS 0.22%) and the knock row.

Pattern observations (pattern-observer): no new implicit standards — the work followed session-api-layering (plain-data Snapshot growth), conversion-as-data (Alt mirrors Factor/Bias/Lookup), and the BLMBody idiom for terminal content builders (inline ANSI, no positioning codes). One naming note: pure view builders now live in `pkg/stream/statusviews.go`; if more views accumulate, consider a `pkg/stream/view` split (not needed yet — consolidate-over-accrete).
