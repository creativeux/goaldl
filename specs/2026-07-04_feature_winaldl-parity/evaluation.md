<!-- SDA: v1.0 -->
# Evaluation: WinALDL Parity — Phase 2 (Tune)

Evaluator: QA (skeptical). Scope: uncommitted working-tree changes vs `HEAD`.

## Verification commands (actual output)

| Command | Result |
|---------|--------|
| `go build ./...` | exit 0, no output |
| `go vet ./...` | exit 0, no output |
| `gofmt -l pkg cmd` | printed nothing (clean) |
| `go test -race -count=1 ./...` | all packages `ok` (cmd/goaldl, blm, decoder, ecm, stream) |
| `go test ./pkg/decoder -run TestGolden -count=1` | `ok` (no `-update`) |
| `go run ./cmd/goaldl blm …/drive_4800.raw` | `Recorded 469 into BLM cells (skipped 40 open-loop, 126 block-learn-disabled)` |
| `go run ./cmd/goaldl monitor …/drive_4800.raw -blm -speed 0` | grid renders, exit 0 |
| `go test ./cmd/goaldl -run TestTUIDriveFixtureEndToEnd -v` | `PASS` |

Forbidden-package check: `git status --short` shows only `cmd/goaldl/tui.go`, `cmd/goaldl/tui_test.go`, `pkg/stream/blmview.go`, `pkg/stream/table.go` modified, plus new untracked `pkg/stream/gridviews.go` / `gridviews_test.go` (and docs/specs). **None of `pkg/stream/session.go`, `pkg/blm/`, or `pkg/ecm/` were touched.**

### Acceptance Criteria
| Criterion | Verdict | Evidence |
|-----------|---------|----------|
| 1. INT grid, closed-loop gated, heatmap + active cell + status | ✅ PASS | `accumulate` adds to `intGrid` only under `if ft.ClosedLoop` (tui.go:321-323); `INTBody` (gridviews.go:71-84) shows "OPEN LOOP — integrator frozen" when not closed, highlights active cell via `gridHeat`. `TestINTBodyGating` + `TestTUIGridAccumulation` confirm. |
| 2. O2 grid ungated, cells 2-dec, status/file 3-dec | ✅ PASS | `accumulate` adds `Sensors["oxygen_sensor"]/1000` on every parsed frame, no gate (tui.go:324); `O2Body` passes `prec=2` for cells, `%.3f` status (gridviews.go:92-97); `writeO2File` uses `RenderFloat(...,3)` (tui.go:405). `TestO2BodyPrecision` asserts exactly one `0.834` (status) and a `0.83` cell. |
| 3. Save (`s`) writes 3 files; BLM/INT have Correction, O2 does not | ✅ PASS | `saveGrids` writes `goaldl_<ts>_{BLM,INT,O2}.txt`; `writeTrimGridFile` emits Samples+Wide Average+Correction, `writeO2File` omits correction. `TestSaveGrids` verifies filenames and correction presence/absence. |
| 4. Clear (`c`) clears active grid; resets extrema on sensors; no-op elsewhere | ✅ PASS | `clear()` switches on `m.active` (tui.go:347-363); Flags/Codes/Raw fall through returning unchanged `m.notice`. `TestTUIClearIsolation` confirms isolation + extrema reset. |
| 5. Sensor tab 6-col SENSOR·RAW·VALUE·MIN·MAX·ALT, primary-unit extrema, reset on `c` | ✅ PASS | `SensorTableExtrema`→`renderTableExtrema` (table.go:199-241); extrema stored from `s.Sensors` (primary unit) and formatted via `formatNum(v,p.Unit)`. `TestSensorTableExtrema` + `TestTUIViewPerTab` (MIN/MAX headers). |
| 6. Persistent loop line on every tab; badge colors; recording dots; holds across bad frame | ✅ PASS | `loopStatusLine` rendered in `View` between header and body for all tabs; `LoopBadge`/`LoopStatus` (gridviews.go:99-138) give CLOSED/OPEN/`LOOP —`; dim style before first good frame. `TestTUILoopLineHoldsLastGood` proves hold across a bad frame; `TestLoopStatus` covers all 4 states + dots. |
| 7. Tabs reorder Sensors·BLM·INT·O2·Flags·Codes·Raw; keys 1-7; tab/arrows cycle | ✅ PASS | `view` enum + key handlers 1-7 + tab/shift-tab modulo `viewCount` (tui.go:181-277). `TestTUITabSwitching` walks all keys and wrap-around. |
| 8. No regression: monitor 4-col; blm/monitor -blm unchanged (469); Snapshot/Session/blm/ecm unchanged; goldens byte-identical | ✅ PASS | `blm` still prints 469; `monitor -blm` renders; `SensorTableExtrema(nil)` falls back to 4-col `SensorTable` (unchanged monitor path, test-asserted equal); forbidden packages untouched; TestGolden passes without `-update`. |

### Standards Compliance
| Standard | Verdict | Notes |
|----------|---------|-------|
| decoder/byte-value-decoding | ✅ | Decode path untouched; goldens byte-identical (TestGolden passes, no `-update`). |
| decoder/raw-data-policy | ✅ | No filtering added to transport. Gating (INT closed-loop, BLM closed-loop+enable) is consumer-side in the TUI model, not in the decode/emit path. BLM binning reads raw bytes faithfully (matches 469). Quality signals (ParseOK/PROMOK) remain fields, not drop-filters — the raw history still takes every frame. |
| architecture/session-api-layering | ✅ | New views consume `stream.Snapshot` (`Sensors`+`FuelTrim`); grids are `pkg/blm.NewDefault()` generic accumulators; frame-layout knowledge stays in `pkg/ecm` (INTBody/O2Body/BLMBody call `ecm.FuelTrimSample`). Presentation added in `pkg/stream` view builders + TUI, not in the core. No `Snapshot`/`Session` field changes. |
| testing/golden-fixtures | ✅ | End-to-end test drives the real `drive_4800.raw` fixture through a live Session; BLM cross-checked to the `blm` command's 469. Golden untouched. |
| go/tooling | ✅ | gofmt/vet/build/test-race all clean. No new dependencies (gridviews.go imports only fmt/math/strings/blm/ecm; tui.go adds only stdlib io/path/filepath/time). |
| consolidate-over-accrete (philosophy) | ✅ | Grid rendering consolidated: `BLMBody` refactored to delegate to the shared `gridHeat`; INT/O2 reuse the same renderer and the same `blm.Grid` rather than forking parallel logic. New capability grows as a Session consumer. |
| ground-truth-first (philosophy) | ✅ | Acceptance is anchored to the committed real capture (469 BLM samples, full-drive replay), not synthetic data. MAP transfer used is the WinALDL-verified one in `ecm`. |

### Persona Reviews

**Product Manager** — Scope matches the agreed 2.1–2.4 + Decision 4 exactly; nothing out-of-scope crept in (no spark grid, no CSV/recording toggles, no replay-speed keys, no `serve`). User value is clear: the dashboard becomes a live three-grid tuning instrument with save/clear and always-on min/max, and the loop line answers the operator's constant question ("is the grid I'm looking at actually recording right now?"). Every criterion is objectively testable and is in fact covered by a named test. Acceptance is well-formed.

**Architect** — Clean layering. The forbidden seam (`Snapshot`/`Session`/`blm`/`ecm`) is genuinely untouched; INT/O2 are the same generic `blm.Grid` on default axes with different value sources, so no ECM specifics leaked into `pkg/blm`. The `BLMBody→gridHeat` refactor removes duplication rather than adding a parallel path, and `SensorTableExtrema(nil)` preserves the monitor contract with a fallback instead of a fork. No new dependencies. One small debt worth noting: `save()` writes `goaldl_*.txt` into the current working directory, which the repo's `.gitignore` only ignores for `*.raw`/`*.csv` — saving from a session run at the repo root leaves untracked files in `git status`. Non-blocking housekeeping.

**QA** — Coverage is strong and the risky edges are explicitly tested: bad-frame gating (decoded views hold `lastGood`, raw view and counters still advance), open-loop freezing of BLM/INT while O2 keeps accumulating, clear isolation, loop-line hold-across-bad-frame, and a full-fixture end-to-end that cross-checks BLM==469 / INT>BLM / O2≥INT with every tab rendering. On the specific concern (c): `accumulate` binds `ft := s.FuelTrim` and calls `m.grid.Add` under `ft.Recordable()` **before** the `if !s.ParseOK { return }` guard — but this cannot bin spurious zeros: `Recordable()` requires MWAF1 bits 7&1 set, `FuelTrimSample` returns a zero (non-Recordable) value for any frame too short to parse, and the value binned is the real byte, faithfully. This pre-parse placement is deliberate and required — it is exactly how the `blm` command and `BLMView` accumulate, which is why the TUI's count matches 469; moving the BLM add after the ParseOK gate would risk diverging from the reference count. INT/O2/extrema, which read the decoded `Sensors` map, are correctly behind the ParseOK early-return so they never read an empty map. Gating distinction (a) is correct: INT `if ft.ClosedLoop`, BLM `ft.Recordable()` = ClosedLoop&&BLMEnabled, and the recording dots mirror this (`intOn = ClosedLoop`, `blmOn = ClosedLoop&&BLMEnabled`, `o2On = hasGood`).

### Issues Found
1. **Saved grid files are not gitignored.** `save()`→`saveGrids(".", …)` writes `goaldl_<ts>_{BLM,INT,O2}.txt` to the working directory; `.gitignore` covers only `/*.raw` and `/*.csv`, so saving from a TUI launched at the repo root leaves untracked `.txt` files in `git status`. Location: `cmd/goaldl/tui.go:338` / `.gitignore`. Severity: **note** (housekeeping; consider adding `/*.txt` or a `goaldl_*` pattern, or documenting the output dir).
2. **BLM accumulation is intentionally pre-ParseOK, creating a subtle asymmetry with the loop dot.** The BLM grid can add from a frame that failed `def.Parse` yet is `Recordable`, whereas the `blm` recording dot (`blmOn`) requires `hasGood`. For real 20-byte frames the two coincide (the 469 match confirms no divergence), and the placement is required for parity, so this is documented behavior rather than a defect. Location: `cmd/goaldl/tui.go:313-323`. Severity: **note** (no observed impact; flagged for awareness).

No blocking or warning-level issues found.

### Overall Verdict
**PASS** — all 8 acceptance criteria met, all standards satisfied, no forbidden packages modified, decoder goldens byte-identical, and the full build/vet/gofmt/test-race gate plus the 469-sample cross-check and end-to-end fixture test all pass. Only two non-blocking notes (untracked `.txt` output location; documented pre-parse BLM binning).
