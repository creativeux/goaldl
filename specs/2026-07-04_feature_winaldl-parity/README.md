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

## Phase 2 (Tune) — spec-feature (2026-07-04)
- 2026-07-04: Resumed to spec **Phase 2 (Tune)** — plan.md steps 2.1–2.4 (INT grid, O2 grid, in-TUI Save/Clear, sensor Min/Max). User confirmed Phase 2 as the spec target over Phase 3 / serve adapter.
- 2026-07-04: Three UX decisions taken with the user: (1) **group the grids** — tabs reorder to `1 Sensors · 2 BLM · 3 INT · 4 O2 · 5 Flags · 6 Codes · 7 Raw` (grids adjacent; Flags/Codes/Raw renumber); (2) **Save writes all three grids at once** (one `s`, shared timestamp, three files); (3) **always-on MIN/MAX columns** on the sensor tab, `c` resets extrema.
- 2026-07-04: **Spec refinement (user UX)** — loop state (Open/Closed) promoted to **persistent chrome on every tab**, since it governs whether the BLM/INT grids are accumulating and thus whether those tabs have value at all. Added `stream.LoopStatus(ft, hasGood)` pure builder → a fixed status line under the tab bar showing the loop badge + per-grid ●/○ recording dots, derived from the existing `FuelTrim` (no new Snapshot data). `BLMBody` left unchanged (shared with the non-tabbed `monitor -blm` view); minor BLM/INT-tab redundancy accepted. Standards gate verdicts unchanged (still no Snapshot/ecm/blm change; pure builder) — no re-gate needed.
- 2026-07-04: Wrote [spec-phase2.md](spec-phase2.md). Key architectural finding: **Phase 2 needs no `Snapshot`/`Session`/`pkg/blm`/`pkg/ecm` change** — INT/O2 grids and extrema are consumer-side state accumulated from the existing Snapshot stream (`Sensors` + `FuelTrim`); `integrator`/`oxygen_sensor` are already parsed. INT gates on ClosedLoop only (distinct from BLM's ClosedLoop+BLMEnabled); O2 ungated. Work is confined to `pkg/stream` view builders (`gridHeat` refactor + `INTBody`/`O2Body`; `SensorTableExtrema`) and `cmd/goaldl/tui.go` (model + save). Decode path untouched.

## Persona Review (spec-phase2.md)
- **Product Manager**: Continues the at-the-car tuning story; INT/O2/save close the "needs a post-hoc `blm` run" gap (success criterion 2). Scope matches the agreed Phase 2 cut; out-of-scope explicit. Save-all-grids and always-on Min/Max are the user's own choices; empty grids write harmlessly. Acceptance criteria testable per view. **Approve.**
- **Architect**: No `Snapshot`/`Session`/`blm`/`ecm` changes — presentation + consumer-side accumulation only; session-api-layering upheld (serve adapter reconstructs grids from the same stream). `blm.Grid` reused (no new dep); `gridHeat` consolidates rather than forks `BLMBody`; 4-column `SensorTable` preserved (no `monitor` regression). Noted pre-existing debt: grid accumulation is duplicated between TUI `Update` and monitor's `BLMView`; Phase 2 adds INT/O2 to the TUI only, widening it slightly — acceptable, consolidate if `monitor` later grows grids. **Approve.**
- **QA**: Every new view has a named oracle (drive fixture / arithmetic); edge cases enumerated (no closed-loop, never-ParseOK, empty save, narrow terminal, timestamp collision, O2 mV→V scaling). Added requirements: (a) an INT test must assert the ClosedLoop-only gate *distinctly* — a `ClosedLoop && !BLMEnabled` frame bins INT but not BLM; (b) explicitly re-run decoder goldens to confirm byte-identical. Both folded into §6. **Approve.**

Synthesis: all personas approve → proceed.

## Standards Gate Report (Phase 2 pre-implementation)
| Standard | Scope | Severity | Verdict |
|---|---|---|---|
| decoder/byte-value-decoding | decoder | must | ✅ PASSES (decode path untouched) |
| decoder/raw-data-policy | decoder | must | ✅ PASSES (grid/extrema gating is view-level on ParseOK/ClosedLoop; PROM never filters; no decode-path filtering) |
| testing/golden-fixtures | testing | should | ✅ PASSES (decoder goldens byte-identical; new accumulation tests rooted in drive_4800.raw) |
| architecture/session-api-layering | architecture | must | ✅ PASSES (no Snapshot/Session change; grid values read from Sensors/FuelTrim; offsets stay in pkg/ecm; blm stays generic) |
| go/tooling | go | should | ✅ PASSES (no new deps; gofmt/vet/test -race gate) |
| philosophy: consolidate-over-accrete | core | — | ✅ (gridHeat shared, BLMBody delegates; SensorTable 4-col path reused) |
| philosophy: ground-truth-first | core | — | ✅ (conversions already log-verified; new tests anchored to fixtures/arithmetic) |

Gate decision: **PROCEED to implement-feature.**

## Phase 2 — implement-feature (2026-07-04)
- 2026-07-04: Resumed for implementation. Read spec-phase2.md; Active Capabilities for this phase: Go toolchain (build/vet/gofmt/test -race), replay-driven TUI tests against `drive_4800.raw`. No browser/DB/PM tools needed (Go stdlib + existing patterns; no external docs). Wrote the Phase 2 task breakdown in [tasks.md](tasks.md) (8 steps + verify) — user approved.
- 2026-07-04: **Phase 2 implemented** — all 8 tasks + verify complete ([tasks.md](tasks.md)). Confirmed no `Snapshot`/`Session`/`pkg/blm`/`pkg/ecm` change; decode path untouched (goldens byte-identical).

### Files changed (Phase 2)
- `pkg/stream/gridviews.go` (new) — `gridHeat` shared heatmap renderer (extracted from BLMBody), `INTBody` (closed-loop gated), `O2Body` (ungated, 3-dec volts), `LoopBadge`/`LoopStatus` (persistent loop/recording line, pure builders over `FuelTrim`).
- `pkg/stream/blmview.go` — `BLMBody` refactored to delegate to `gridHeat` (behavior identical; still used by `monitor -blm`).
- `pkg/stream/table.go` — `Row` gained Min/Max; `BuildRowsExtrema` + `renderTableExtrema` + `SensorTableExtrema` (6-col dashboard table, nil-extrema → 4-col `SensorTable` fallback so `monitor` is untouched).
- `cmd/goaldl/tui.go` — view enum regrouped (Sensors·BLM·INT·O2·Flags·Codes·Raw), keys 1–7; model fields `intGrid/o2Grid/mins/maxs/hasExtrema/notice`; `accumulate` (3 grids + extrema, consumer-side), `save`/`clear`/`saveGrids`/`writeTrimGridFile`/`writeO2File`; persistent `loopStatusLine()` (green/amber/dim badge); footer notice + updated legend.
- Tests: `pkg/stream/gridviews_test.go` (new); `cmd/goaldl/tui_test.go` extended (grid accumulation, clear isolation, save format, loop-line hold, **end-to-end drive-fixture** run cross-checking BLM==469 vs the blm command); existing tab-switch/view tests updated for the 7-tab layout.

### Verify (implement-feature)
- `go test -race ./...` all green; `go vet` + `gofmt -l` clean.
- Decoder goldens byte-identical (`TestGolden` re-run, no `-update`) — decode path untouched.
- Non-regression: `monitor -blm` renders through the shared `gridHeat`; `blm` command still records 469 closed-loop samples / 27 trusted cells over the drive fixture.
- End-to-end: the dashboard model driven over all 635 drive-fixture frames via a real `Session` — BLM grid 469 (matches the `blm` command), INT > BLM (closed-loop is the looser gate), O2 ≥ INT (ungated), all 7 tabs render with the loop line present.

### Post-implementation tweak (user feedback)
- 2026-07-04: **O2 grid legibility** — user reported the live O2 heatmap cells run together. Cause: a 3-decimal value (`0.834`) fills the full 5-wide cell, leaving no gutter (BLM/INT breathe only because their integer values leave leading spaces). Fix: live O2 grid cells render to **2 decimals** (`" 0.83"` → leading-space gutter); the current-reading status line and the **saved** O2 file keep full **3-decimal** precision. One-line change (O2Body `prec` 3→2); `TestO2BodyPrecision` updated (3-dec only in status, cells 2-dec); verified against the drive fixture (grid columns now separated, active-cell highlight intact). Spec §1/§2 updated.

### Verify-feature (2026-07-04)
- 2026-07-04: Resumed for verify-feature. Assembled a self-contained [evaluation-brief.md](evaluation-brief.md) (Sections A–F: requirements, Phase 2 acceptance criteria, changed files, verify commands, standards, personas). Spawned a fresh evaluator with no implementation context.
- 2026-07-04: **Evaluator verdict: PASS** ([evaluation.md](evaluation.md)). All 8 acceptance criteria PASS with named test coverage; all 5 standards + both core philosophies satisfied; PM/Architect/QA persona reviews all positive. Independently confirmed: forbidden packages (`session.go`/Snapshot/Session, `pkg/blm`, `pkg/ecm`) untouched; decoder goldens byte-identical (no `-update`); `blm` still records 469; INT-vs-BLM gating correct (INT `ClosedLoop`, BLM `Recordable()`); `accumulate` cannot bin spurious zeros (INT/O2/extrema behind the `!ParseOK` early-return; pre-parse BLM add yields a non-Recordable zero for short frames — deliberate parity with the `blm` command). Two **non-blocking notes**: (1) saved `goaldl_*.txt` grid files weren't gitignored; (2) documented pre-parse BLM-binning asymmetry (no observed impact).
- 2026-07-04: **Note 1 resolved** — added `/goaldl_*.txt` to `.gitignore` (saved grid tables). Note 2 needs no action (documented intentional parity).

### Spec retrospection
- Reconciled two implementation drifts into [spec-phase2.md](spec-phase2.md): (a) grid-body builders thread the current value in — `INTBody(g, ev, minCount, intVal)` and `O2Body(g, ev, o2Volts)` (from `Snapshot.Sensors`) rather than re-deriving it, since `ecm.FuelTrim` carries only BLM (this is what kept `pkg/ecm` unchanged); (b) the file writer is named `writeTrimGridFile` (not `writeBLMLikeFile`). Also fixed the test-plan row's writer reference.
- Standards audit: `architecture/session-api-layering.md` cites `SensorTable`/`BLMBody`/`Renderer`/`BLMView` as presentation examples — all still exist with unchanged signatures (BLMBody refactored internally only); no stale code examples in `product-knowledge/standards/`.

### Test synchronization
- Stale references: none — the feature's tests reference only current/new APIs (no deleted or renamed modules).
- Fakes/doubles: none introduced — tests drive the real `ReplayProvider` + `Session` + real `ecm` definition against the committed `drive_4800.raw` fixture.
- New public method coverage: `INTBody` (TestINTBodyGating), `O2Body` (TestO2BodyPrecision), `LoopStatus`/`LoopBadge` (TestLoopStatus), `SensorTableExtrema` (TestSensorTableExtrema); `saveGrids`/`writeTrimGridFile`/`writeO2File` (TestSaveGrids); `accumulate`/`save`/`clear`/`loopStatusLine` via the model tests + end-to-end. `BuildRowsExtrema` is exercised transitively through `SensorTableExtrema` (thin builder, fully covered by the render assertions).
- Sibling comparison: closest siblings are `blmview_test.go`/`statusviews_test.go` — the dim-below-`minCount` path they cover is now the shared `gridHeat`, exercised through the BLM tests (INT reuses it; O2 fixes `minCount=1`, never dims). No coverage gap.
- Regression: `go test -race -count=1 ./...` all green; `go vet` + `gofmt -l pkg cmd` clean.

### Completion (2026-07-04)
- **Phase 2 (Tune) verified and closed.** Observability updated: `PROJECT_STATUS.md` Current Focus marks Phase 2 shipped+verified and Phase 3 next-up; Recent Changes records the verification. `ROADMAP.md` populated with the real Phase 0–4 structure (Phase 2 ✅). `CLAUDE.md` dashboard description updated (5→7 tabs, INT/O2 grids, loop-state line, save/clear, new view builders). `.gitignore` gains `/goaldl_*.txt`. Pattern-observer left one pending UX philosophy for `recombobulate` review.
- **Next**: Phase 3 (session UX — recording toggle, replay pause/speed keys, spark grid, CSV toggle) via `plan-feature` → `spec-feature`, and/or the `serve` adapter to exercise the Session API on a non-terminal front-end. Working-tree changes are uncommitted (commit when ready).

### Pattern observations (pattern-observer)
- Logged one **UX philosophy** (pending) in `product-knowledge/observations/observed-philosophies.md`: *operating state that gates a view's usefulness belongs in persistent chrome, not inside the gated view* — from the user's loop-status correction. No new enforceable standards; the work followed the established idioms (session-api-layering: grids/extrema are consumer-side over the existing Snapshot; BLMBody-style pure content builders with inline ANSI; blm.Grid reuse; `RenderInt/RenderFloat` for save files). `gridviews.go` is the anticipated `pkg/stream/view`-style split staying ahead of accretion — still one package, acceptable.

## Phase 3 (Session UX) — spec-feature (2026-07-04)
- 2026-07-04: Resumed to spec **Phase 3 (Session UX)** — plan.md steps 3.1–3.4 (recording toggle, replay pause/speed keys, spark grid, CSV toggle). User confirmed Phase 3 as the spec target over the `serve` adapter (which needs `plan-feature` first). Scope also carries the deferred **editable save filename** request from Phase 2 (user memory: `s` should prompt, default `goaldl_<ts>`, not auto-name silently).
- 2026-07-04: Three UX decisions taken with the user: (1) **filename prompt on all three file-producing actions** — `s` save, `r` record-start, `d` CSV-start each open an inline prompt pre-filled with the timestamped default (Enter accepts, Esc cancels); (2) **spark grid uses the WinALDL spark axes** — RPM 400–3600/400 × MAP 30–100/5, deliberately finer than the trim grids; (3) **Spark is tab 5, grouped with the grids** — `1 Sensors · 2 BLM · 3 INT · 4 O2 · 5 Spark · 6 Flags · 7 Codes · 8 Raw` (Flags/Codes/Raw renumber to 6/7/8).
- 2026-07-04: Wrote [spec-phase3.md](spec-phase3.md). Key architectural findings: **`Snapshot`/`Session`/`pkg/ecm` unchanged again** — spark accumulation + knock-delta differencing are consumer-side over the existing stream (`knock_count` already parsed); recording and replay controls land on the **providers** (below the Session facade): a new concurrency-safe `stream.RecordSink` slots into the existing `SerialProvider.Sink` field (provider code untouched, write errors detach instead of killing the live session), and `ReplayProvider` gains mutex-guarded `SetPaused`/`SetSpeed` with re-anchored pacing (speed changes are never retroactive). `pkg/blm` gains two tiny generic additions anticipated by plan.md: `Grid.Sum()` and `NewSpark()`. `gridHeat` generalizes to take the cell-values matrix (BLM/INT/O2 pass `Average()`, Spark passes `Sum()`); `LoopStatus` gains a SPARK dot (ungated, same condition as O2). CSV toggle reuses `frameCSV` verbatim with `monitor -csv`'s ParseOK-only row gating. Filename prompt is a hand-rolled line editor — **no new dependencies**; files are created `O_CREATE|O_EXCL` so a name collision keeps the prompt open instead of overwriting.

## Persona Review (spec-phase3.md)
- **Product Manager**: Scope matches the agreed Phase 3 cut plus the user's own deferred filename-prompt request (not creep — it was earmarked for Phase 3); out-of-scope explicit (§9: Dash/config/multi-ECM/serve; monitor stays pre-declared flags). At-the-car value is real: no more deciding to record before launching, knock finally visible per cell. Prompt-while-driving friction is mitigated by the pre-filled default + Enter. Acceptance criteria testable per feature. **Approve.**
- **Architect**: Session facade untouched for the third phase running; controls placed on providers is the right seam (a `serve` adapter gets the same handles). No new deps (prompt hand-rolled). `pkg/blm` additions are generic (Sum; NewSpark follows the DefaultRPM/DefaultMAP display-axes precedent). `gridHeat` generalized, not forked. Noted divergence, accepted: TUI recording fails soft (detach + notice) while `monitor -o` fails loud — different contracts (interactive toggle vs pre-declared flag), documented in spec §1. `saveGrids` signature change is cmd-internal. Model state keeps growing (prompt/replay/recSink/csvLog) — acceptable for the interactive face; revisit if a ninth concern lands. **Approve.**
- **QA**: Every feature has a named oracle (crafted writers, injectable clock, drive fixture, temp-dir fs assertions); edge cases enumerated (§7: counter reset/wrap, zero-knock fixture, no-op key combos, sink write error, prompt-at-stream-end, quit cleanup, collision, pause+CSV). Required and folded in: the prompt test must assert `q` types into the buffer rather than quitting (only ctrl+c exits while prompting) — spec §8 updated. Confirmed the end-to-end row treats a zero-knock fixture as a valid assertion (independent recomputation, not a vacuous pass), and regression rows pin decoder goldens + untouched `monitor`/`blm` paths. **Approve.**

Synthesis: all personas approve → proceed.

## Standards Gate Report (Phase 3 pre-implementation)
| Standard | Scope | Severity | Verdict |
|---|---|---|---|
| decoder/byte-value-decoding | decoder | must | ✅ PASSES (decode path untouched; mid-stream capture relies on the decoder's own 0x1FF resync) |
| decoder/raw-data-policy | decoder | must | ✅ PASSES (no decode-path filtering; CSV ParseOK-only rows are consumer-level, monitor precedent; knock-reset spurious delta accepted and documented, not filtered) |
| testing/golden-fixtures | testing | should | ✅ PASSES (goldens byte-identical, no `-update`; new tests rooted in drive_4800.raw + crafted frames with named oracles) |
| architecture/session-api-layering | architecture | must | ✅ PASSES (Snapshot/Session unchanged; RecordSink/replay controls sit on providers below the facade; frame-layout knowledge stays in pkg/ecm; pkg/blm stays generic — Sum/NewSpark carry no ECM specifics) |
| go/tooling | go | should | ✅ PASSES (zero new dependencies — prompt hand-rolled; gofmt/vet/test -race gate unchanged) |
| philosophy: consolidate-over-accrete | core | — | ✅ (gridHeat generalized rather than forked; frameCSV reused verbatim; RecordSink is one implementation behind the existing Sink seam) |
| philosophy: ground-truth-first | core | — | ✅ (spark axes from the WinALDL GIFs; knock wrap arithmetic tested against crafted sequences; end-to-end recomputed independently over the committed fixture) |

Gate decision: **PROCEED to implement-feature.**

## Phase 3 — implement-feature (2026-07-04)
- 2026-07-04: Resumed for implementation (user approved the 11-step breakdown in [tasks.md](tasks.md)). Active Capabilities unchanged from Phase 2: Go toolchain (build/vet/gofmt/test -race), replay-driven TUI tests against `drive_4800.raw`; no browser/DB/PM tools needed.
- 2026-07-04: **Phase 3 implemented** — all 10 tasks + verify complete. Confirmed `Snapshot`/`Session`/`pkg/ecm` untouched; decode path untouched (goldens byte-identical, no `-update`); no new dependencies.

### Files changed (Phase 3)
- `pkg/blm/blm.go` — generic `Grid.Sum()` accessor; `SparkRPM`/`SparkMAP`/`NewSpark()` (WinALDL spark display axes 400–3600/400 × 30–100/5).
- `pkg/stream/record.go` (new) — `RecordSink`: mutex-guarded switchable raw-capture tee; `Write` never errors the provider (a target write error detaches + sets sticky `Err`); `Set` swaps targets returning the old one + its byte count.
- `pkg/stream/replay.go` — runtime `SetPaused`/`Paused`/`SetSpeed`/`CurrentSpeed` on `ReplayProvider`; pacing rewritten around re-anchoring (speed changes apply from the current position, never retroactive; pause freezes the data position) with waits sliced ≤100 ms; `Speed==0` (unpaced) leaves controls inert. Exported `Speed` field semantics preserved (initial rate).
- `pkg/stream/gridviews.go` — `gridHeat` generalized to take the cell-values matrix; `SparkBody` (values=`Sum()`, ungated, prec 0, KNOCK_CNT status, WinALDL-axes header); `LoopStatus` gains a SPARK dot (== O2's ungated condition). `blmview.go` — `BLMBody` passes `Average()` (behavior identical).
- `cmd/goaldl/tui.go` — 8 tabs (Spark at 5, keys 1–8); spark knock-delta accumulation (baseline first frame, mod-256 wrap, `c` keeps baseline); modal filename prompt on `s`/`r`/`d` (hand-rolled line editor, `O_CREATE|O_EXCL`, collision keeps prompt open with hint); `saveGrids(dir, base, …)` writes 4 files incl. `writeSparkFile`; `r` recording toggle over a `RecordSink` wired in `cmdTUI` (live only; sink-error auto-stop; `closeOutputs` after program exit); `d` CSV toggle reusing `frameCSV` (ParseOK rows only — monitor parity); `space`/`+`/`=`/`-` replay keys (clamped 0.25×–16×, no-op notices on live/unpaced); footer gains `● REC name bytes` / `CSV name rows` / `⏸ PAUSED` / `N×` chrome segments.
- Tests: `pkg/blm/blm_test.go` (Sum, spark axes), `pkg/stream/record_test.go` (new: discard/count/swap/error-detach/concurrent -race), `pkg/stream/stream_test.go` (pause-resume, non-retroactive speed change, unpaced-inert — injectable clock), `pkg/stream/gridviews_test.go` (SparkBody sum-not-average, LoopStatus 4 states × 5 dots), `cmd/goaldl/tui_test.go` (prompt editing incl. digits/`q` typing + esc + collision, spark deltas incl. 250→4 wrap + clear-keeps-baseline, recording toggle live/replay, CSV toggle ParseOK-row gating, 4-file save + exclusive-create refusal, 8-tab layout, end-to-end with independent spark recomputation).

### Verify (implement-feature)
- `go test -race -count=1 ./...` all green; `go vet` + `gofmt -l pkg cmd` clean.
- Decoder goldens byte-identical (`TestGolden`, no `-update`) — decode path untouched.
- Non-regression: `blm` command still records 469 over the drive fixture; `monitor -blm` renders through the shared `gridHeat`; existing replay pacing/cancel tests pass unmodified against the rewritten `Run`.
- End-to-end: dashboard model driven over all drive-fixture frames via a real `Session` — BLM==469, INT>BLM, O2≥INT, spark total matches the independent knock-delta recomputation, all 8 tabs render with the loop line present.
- One implementation fix found by the tests: the save path now passes dir `""` so a typed absolute/relative path in the prompt is honoured verbatim (`filepath.Join(".", "/abs/…")` had silently relativized it).

### Pattern observations (pattern-observer)
- No new user-sourced standards or philosophies this session (decisions were taken in spec Q&A and followed existing idioms: providers own transport controls below the Session facade; pure content builders; consumer-side accumulation; `RenderInt`/`RenderFloat` save files). One design distinction documented in spec §1 rather than logged as an observation, since it was agent-proposed: *interactive toggles fail soft (detach + notice); pre-declared flags fail loud* — candidate for a standard if it recurs.

### Post-implementation tweak (user feedback)
- 2026-07-04: **No-op warnings self-expire** — user reported that pressing `r` during replay leaves the "recording needs a live source (-p)" warning in the footer indefinitely; it should disappear after 3 seconds. Applied to the whole class of no-op key warnings (record-on-replay, pause/speed-on-live, unpaced `-speed 0`): `warn()` sets the notice and arms a `tea.Tick(noticeTTL)` carrying a **notice sequence number**; every notice change bumps the sequence (all sites now go through `setNotice`), so a stale timer expiring after a newer notice (e.g. a save confirmation) is a no-op instead of wiping it. Action notices (saved/recording/stopped/cleared) remain until replaced, as before. New `TestTUINoticeExpiry` covers fire-clears and stale-timer-preserves; full `-race` suite green; spec §7/§8 updated.

### Verify-feature (2026-07-04)
- 2026-07-04: Resumed for verify-feature. Evaluation brief re-assembled for Phase 3 ([evaluation-brief.md](evaluation-brief.md), Sections A–F: requirements + 3 user decisions + notice-expiry feedback, 10 acceptance criteria, changed-file map, verify commands incl. forbidden-seam diff check, standards, personas). Spawned a fresh evaluator with no implementation context (filesystem-only handoff).
- 2026-07-04: **Evaluator verdict: PASS** ([evaluation.md](evaluation.md)). All 10 acceptance criteria PASS with named evidence; all 5 standards + both core philosophies satisfied; PM/Architect/QA persona reviews positive. Independently confirmed: forbidden seam (`session.go`/`pkg/ecm`/`pkg/decoder`/`go.mod`) diff **empty**; goldens byte-identical; `blm` still 469; full suite green under `-race`; pacing arithmetic proven to ±1 ms via injectable clock against the real fixture. **Two warnings, both fixed same-session**: (1) stale "see 7 Raw" waiting message → "8 Raw" (`tui.go`); (2) stale CLAUDE.md (7 tabs / 3-grid save / "Phase 3 next") → rewritten for the 8-tab dashboard, session-UX keys, and Phases 2+3 shipped. **Five notes, accepted**: CSV stat-then-create TOCTOU (spec-sanctioned, §2), snapshot-driven sink-error surfacing (~1.2 s exposure), partial file set on mid-save non-collision error, no direct `closeOutputs` test (→ closed, see test sync), esc-cancel assertion scope.

### Spec retrospection
- Reconciled three implementation drifts into [spec-phase3.md](spec-phase3.md): (a) `RecordSink.Set` returns `(old io.Writer, written int64)` — the byte count feeds the stop notice; (b) `promptState` carries a `hint` field (collision message, cleared on next edit); (c) the TUI passes `saveGrids` dir `""` so a typed absolute path is honoured verbatim (`filepath.Join(".", "/abs")` would relativize it — bug caught by the prompt tests during implementation).
- Standards audit: `architecture/session-api-layering.md` cites `SensorTable`/`BLMBody`/`Renderer`/`BLMView` — all exist with unchanged signatures (`BLMBody` changed internally only; `gridHeat` is unexported). No stale code examples in `product-knowledge/standards/`.

### Test synchronization
- Stale references: none — the feature's tests reference only current APIs (the old `save()`/timestamp `saveGrids` are gone from tests; no deleted/renamed module imports).
- Fakes/doubles: `failWriter` (record_test.go) fails like a real full disk (error after N bytes) — matches `RecordSink`'s any-error-detaches semantics; replay tests use the established injectable now/sleep, not a fake provider; TUI tests drive the real `ReplayProvider` + `Session` + real ecm definition.
- New public API coverage: `blm.Sum` (TestSum), `blm.NewSpark` (TestNewSparkAxes), `RecordSink` (4 tests incl. `-race` concurrency), `ReplayProvider.SetPaused/Paused/SetSpeed/CurrentSpeed` (3 provider tests + TestTUIReplayKeys), `SparkBody` (TestSparkBody), extended `LoopStatus` (TestLoopStatus). cmd-internal: saveGrids/writeSparkFile (TestSaveGrids), prompt (TestTUIPromptEditing), toggles (TestTUIRecordingToggle/TestTUICSVToggle), notice expiry (TestTUINoticeExpiry).
- Evaluator note 6 **closed**: added `TestTUICloseOutputs` — quit with active recording+CSV detaches the sink before closing, both handles closed, post-quit provider writes safely discarded.
- Sibling comparison: `SparkBody` matches its siblings' (`INTBody`/`O2Body`) coverage shape (status/highlight/dim/legend); replay control tests extend the existing pacing suite over the same fixture; `RecordSink` has no sibling (new seam) — covered directly.
- Regression: `go test -race -count=1 ./...` all green; `go vet` + `gofmt -l pkg cmd` clean; goldens re-confirmed after the "8 Raw" string fix.

### Completion (2026-07-04)
- **Phase 3 (Session UX) verified and closed.** Observability updated: `PROJECT_STATUS.md` marks Phase 3 shipped+verified (serve adapter next); `ROADMAP.md` Phase 3 → ✅ with the as-built line items; `CLAUDE.md` dashboard section rewritten (8 tabs, session-UX keys, provider-level controls, Phases 2+3 shipped). The editable-filename memory item is fulfilled.
- **Next**: the `serve` adapter (HTTP/WebSocket over the same Snapshot stream) via `plan-feature`, and/or Phase 4 deferred items. Working-tree changes are uncommitted (commit when ready).
