<!-- SDA: v1.0 -->
# Tasks: WinALDL Parity Phase 1 (MVP)

Breakdown mirrors plan.md Phase 1 / spec.md; agreed with user 2026-07-04 (MVP cut).

- [x] 1.1a `pkg/ecm`: FlagBit/FlagWord/ErrorCode + status types, DecodeFlags/DecodeCodes; Definition gains FlagWords/ErrorCodes/ByteLabels; AltConversion on Parameter (`flags.go`, `ecm.go`)
- [x] 1.1b `pkg/ecm`: 1227747 tables (MW2/MWAF1/MCU2IO flags, MALFFLG1-3 codes, byte labels, knock_count param, Alt conversions CT°C/KPH/MAP kPa/TPS%) (`gm_1227747.go`)
- [x] 1.1c `pkg/ecm`: MapVoltsToKPa → verified (raw+28.06)/2.71; fueltrim↔flag-table consistency test; log-oracle tests (flags, codes, kPa, TPS%) (`fueltrim.go`, `flags_test.go`)
- [x] 1.1d `pkg/stream`: Snapshot gains Flags/Codes; Session caches def, uses def.Parse; tests over drive fixture (`session.go`, `session_test.go`)
- [x] 1.4a `pkg/stream`: BuildRows/SensorTable ALT column; Renderer takes a (calibratable) Definition; def-level Parse added to ecm (`table.go`, `stream_test.go`)
- [x] 1.2 TUI codes view (set prominent, unset dim, not-used hidden unless set) (`statusviews.go` CodesBody, `tui.go`)
- [x] 1.3 TUI flags view (3 groups, checkboxes, SetLabel) (`statusviews.go` FlagsBody, `tui.go`)
- [x] 1.5 TUI raw history grid (labeled rows × last N frames, newest first, width-clamped to ≤14 cols, 64-frame ring) (`statusviews.go` RawHistory, `tui.go`)
- [x] 1.6 TUI heartbeat footer (● green/red + ok/bad counts) + ParseOK gating (decoded views render lastGood; raw view never gated) (`tui.go`)
- [x] 1.4b cmd flags `-tps0/-tps100` on TUI + monitor, degenerate-calibration guard with warning (`tui.go` calibratedDef, `monitor.go`)
- [x] V: full suite green incl. `-race` + vet + gofmt; decoder goldens untouched; blm expectations re-derived for the verified kPa transfer — cross-checked against WinALDL's own table (our 1600/40 avg 117.17 vs WinALDL's 117.5 from a different session); monitor exercised end-to-end over the idle fixture (ALT column renders)

---

# Tasks: WinALDL Parity Phase 2 (Tune)

Breakdown mirrors [spec-phase2.md](spec-phase2.md); user decisions 2026-07-04 (group grids · save-all · always-on Min/Max · persistent loop chrome). No `Snapshot`/`Session`/`pkg/blm`/`pkg/ecm` change — presentation + consumer-side accumulation only.

- [x] 2.1 `pkg/stream`: refactored shared `gridHeat(g, ar, ac, minCount, prec, status, legend)` out of `BLMBody` (BLMBody behavior unchanged — substring tests + `monitor -blm` verified); added `INTBody` (ClosedLoop-gated, prec 0) and `O2Body` (ungated, minCount 1, prec 3) in `gridviews.go`
- [x] 2.2 `pkg/stream`: `LoopStatus(ft, hasGood)` + `LoopBadge(ft, hasGood)` pure builders — badge word + BLM/INT/O2 ●/○ recording dots for the four states
- [x] 2.3 `pkg/stream`: `Row` gained Min/Max; `SensorTableExtrema` renders SENSOR·RAW·VALUE·MIN·MAX·ALT; nil extrema → existing 4-col `SensorTable` (monitor path untouched, asserted equal)
- [x] 2.4 `pkg/stream/gridviews_test.go`: gridHeat precision (O2 3dec), INT gating (closed vs open), O2 ungated highlight, LoopStatus/LoopBadge 4 states, extrema table 6-col + nil fallback
- [x] 2.5 `cmd/goaldl/tui.go` model: view enum reordered → Sensors·BLM·INT·O2·Flags·Codes·Raw; keys `1`–`7`; added `intGrid/o2Grid/mins/maxs/hasExtrema/notice`; `accumulate(s)` folds all 3 grids + extrema (BLM via Recordable; INT on ParseOK+ClosedLoop; O2 on ParseOK)
- [x] 2.6 `cmd/goaldl/tui.go` view: INTBody/O2Body/SensorTableExtrema wired; persistent `loopStatusLine()` under the tab bar (green closed / amber open / dim unknown, from lastGood.FuelTrim); footer notice + `1-7/tab · s save · c clear · q quit`
- [x] 2.7 `cmd/goaldl/tui.go` save/clear: `s` → `saveGrids(".", time.Now(), …)` writes 3 timestamped files (`writeTrimGridFile` BLM/INT Samples+Avg+Correction; `writeO2File` O2 Samples+Avg 3dec); `c` context — grid tab clears that grid, Sensors resets extrema, else no-op
- [x] 2.8 `cmd/goaldl/tui_test.go`: grid accumulation + open-loop freeze; clear isolation; save file headers (O2 has no correction); loop line holds lastGood across a bad frame; **end-to-end over the drive fixture** (BLM==469 cross-checks the blm command, INT>BLM, O2≥INT, all 7 tabs render); tab-switch + view tests updated for the new layout
- [x] V: full suite green under `-race`; `go vet` + `gofmt` clean; decoder goldens byte-identical (`TestGolden` re-run, no `-update`); `monitor -blm` + `blm` command unchanged (469 recorded); dashboard driven end-to-end over the fixture in-test
