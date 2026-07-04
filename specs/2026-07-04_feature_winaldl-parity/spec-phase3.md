<!-- SDA: v1.0 -->
# Spec: WinALDL Parity — Phase 3 (Session UX)

Scope: plan.md Phase 3 steps 3.1–3.4 — in-TUI raw-recording toggle, replay pause/speed keys, spark-counts grid, CSV-logging toggle — plus the deferred editable-filename prompt (user request, carried from Phase 2). Phases 1 (Diagnose) and 2 (Tune) are shipped and verified. Authorities unchanged: **A033.ads** for layout, **`data/20250601_111156_LOG.txt`** for conversions, committed captures (`pkg/decoder/testdata/drive_4800.raw`) for accumulation/replay tests, WinALDL view GIFs (`docs/winaldl/`) for the spark-grid shape.

**User decisions (2026-07-04)**:
1. **Filename prompt on all file-producing actions**: `s` (grid save), `r` (start raw recording), and `d` (start CSV log) each open an inline filename prompt pre-filled with the timestamped default (`goaldl_<ts>`); Enter accepts, Esc cancels the action. (Fulfils the deferred "editable save filename" request from Phase 2.)
2. **Spark grid uses the WinALDL spark axes**: RPM 400–3600 step 400 × MAP 30–100 step 5 — parity, and the finer MAP localizes knock better. Deliberately different axes from BLM/INT/O2.
3. **Spark is tab 5, grouped with the grids**: `1 Sensors · 2 BLM · 3 INT · 4 O2 · 5 Spark · 6 Flags · 7 Codes · 8 Raw`. Flags/Codes/Raw renumber to 6/7/8 (keeps the Phase 2 grids-adjacent decision).

## 0. Architecture invariants

- **`stream.Snapshot` and `Session` do NOT change.** Spark accumulation and knock-delta tracking are consumer-side state in the TUI model over the existing `Snapshot` stream (`knock_count` is already a parsed parameter, offset 17). Recording and replay controls live on the **providers** — the layer below `Session` — so the Session facade is untouched and a future `serve` adapter inherits the same seams.
- **`pkg/ecm` does NOT change.** No new frame-layout knowledge anywhere outside it; the spark grid reads `Sensors["knock_count"]` and `FuelTrim.RPM/MapKPa` only. (Differencing the cumulative counter is generic modular arithmetic on an already-parsed value — a consumer decision, same as WinALDL's display layer.)
- **`pkg/blm` gains two tiny generic additions**, anticipated by plan.md ("small delta-counter variant"): a `Sum()` accessor (total of readings per cell — no ECM specifics) and `NewSpark()` (the WinALDL spark display axes, symmetric with `NewDefault()`). Nothing else moves.
- **`pkg/stream` grows below the Session line**: a concurrency-safe `RecordSink` (switchable raw tee for `SerialProvider.Sink` — the provider itself is unchanged) and runtime pause/speed methods on `ReplayProvider`. `gridHeat` generalizes to take the cell-values matrix (internal helper; BLM/INT/O2 pass `Average()`, Spark passes `Sum()`).
- **Decode path untouched** → decoder goldens stay byte-identical; no `-update`.
- **No new dependencies**: the filename prompt is a hand-rolled single-line editor (append/backspace/enter/esc), not `bubbles/textinput`.

## 1. Recording toggle — `r` (step 3.1)

**`stream.RecordSink`** (new, `pkg/stream`) — a concurrency-safe switchable tee, passed once as `SerialProvider.Sink` at construction so the provider needs no change and no lock of its own:

```go
// RecordSink is an io.Writer whose target can be attached and detached while a
// SerialProvider is writing to it, so a live session can start and stop raw
// capture on demand. When no target is set, writes are discarded.
type RecordSink struct { mu sync.Mutex; w io.Writer; n int64; err error }
func (s *RecordSink) Write(p []byte) (int, error) // never returns an error to the provider
func (s *RecordSink) Set(w io.Writer) (old io.Writer, written int64) // swap target; returns the old one to close + bytes written to it
func (s *RecordSink) Active() bool
func (s *RecordSink) Bytes() int64  // bytes written to the current target
func (s *RecordSink) Err() error    // sticky write error, cleared by Set
```

- **`Write` never fails the session**: on a target write error (disk full, pulled USB stick) it detaches the target and records the error; the live dashboard keeps running. The TUI checks `Err()` on each snapshot and shows `recording stopped: <err>` (the file is closed by the TUI when it observes the detach). This is deliberately different from `monitor -o` (pre-declared recording, where dying loudly is right); an interactive toggle must not take the session down with it.
- **Wiring**: `cmdTUI` always constructs the live provider with a `RecordSink` (`&stream.SerialProvider{…, Sink: recSink}`); the model holds `recSink` (nil for replay).
- **Keys / semantics**: `r` with recording off → filename prompt (§5, default `goaldl_<ts>`, `.raw` appended) → `os.OpenFile(name, O_CREATE|O_EXCL|O_WRONLY)` → `recSink.Set(f)`; notice `recording → <name>.raw`. `r` with recording on → `old, n := recSink.Set(nil)`, close old; notice `stopped recording <name>.raw (N bytes)`. On quit, an active recording is closed cleanly after the program exits (in `cmdTUI`, after `Run` returns).
- **Replay source**: `r` → notice `recording needs a live source (-p)`; no prompt.
- **Chrome**: while recording, the footer shows a red `● REC <name>.raw <bytes>` segment (style like `beatBad`), updated per frame from `recSink.Bytes()` — persistent operating state, visible on every tab.
- A capture started mid-stream begins at an arbitrary byte; the decoder resyncs on the next 0x1FF (9×`0x00`) when the file is later decoded. No special handling.

## 2. CSV logging toggle — `d` (step 3.4)

- Reuses `frameCSV` (`cmd/goaldl/csv.go`) verbatim — same header (`time_sec,byte_offset,prom_ok` + one column per parameter) and row format as `decode`/`monitor -csv`, over the TUI's calibrated definition.
- Model holds `csvLog *frameCSV` + its name. `d` off→on: prompt (§5, `.csv` appended) → `newFrameCSV` (created `O_EXCL`-style: stat first, § 5 collision rule) → notice `csv log → <name>.csv`. `d` on→off: `Close()`; notice `stopped csv (<rows> rows)`.
- **Row gating — parity with `monitor -csv`**: a row is written only for `ParseOK` frames (`csv.Write(s.Elapsed.Seconds(), s.Frame.ByteOffset, s.PROMOK, s.Sensors)` in the snapshot handler). Bad frames produce no row (monitor writes only when `ParseFrame` succeeds). `Elapsed` is the data timeline, so replay speed/pause never distorts `time_sec`.
- Works on **both live and replay** (harmless and occasionally useful on replay; `decode` remains the batch path).
- Footer shows `CSV <name>.csv <rows>` while active (dim, next to the REC segment).
- Write errors: `frameCSV` is fire-and-forget per row (same as monitor); OS-level failures surface on `Close`. Accepted parity.

## 3. Replay pause / speed — `space`, `+`/`=`, `-` (step 3.2)

**`ReplayProvider`** gains runtime controls (mutex-guarded; the exported `Speed` field stays the initial value, so existing construction in `monitor`/`tui` doesn't move):

```go
func (p *ReplayProvider) SetPaused(v bool)
func (p *ReplayProvider) Paused() bool
func (p *ReplayProvider) SetSpeed(v float64)   // takes effect from the current position
func (p *ReplayProvider) CurrentSpeed() float64 // initial Speed until first SetSpeed
```

- **Mechanics**: `Run` re-anchors on every control change — it tracks `anchorWall` (clock) and `anchorData` (data timeline) and computes each wait as `(dataElapsed − anchorData)/speed − (now − anchorWall)`, resetting both anchors on speed change and on resume. So a speed change is **not retroactive** (no jump/fast-forward); it only re-paces from here. Waits (and the paused state) are slept in bounded slices (≤100 ms) re-checking ctx/pause/speed, so a toggle takes effect within ~100 ms even mid-wait. `Elapsed` on emitted frames is unchanged (data timeline, per the existing contract).
- **`Speed == 0`** (unpaced, `-speed 0`): runtime controls are inert — `Run` never waits, and the TUI keys show notice `unpaced replay (-speed 0)`.
- **Keys**: `space` toggles pause; `+`/`=` doubles speed, `-` halves it, clamped to **0.25×–16×**; notice shows the new rate (`speed 2×`). On a **live** source all three are no-ops with notice `replay-only`.
- **Chrome**: footer shows the current rate when ≠1× (`2×`) and a bold `⏸ PAUSED` while paused. Pausing stops frame flow, so grids/extrema/CSV naturally freeze; no consumer-side changes needed.
- The TUI model holds `replay *stream.ReplayProvider` (nil when live), passed from `cmdTUI`.

## 4. Spark-counts grid — tab 5 (step 3.3)

**Axes** (`pkg/blm`): `SparkRPM = axis(400, 3600, 400)`, `SparkMAP = axis(30, 100, 5)`, `NewSpark()` — the WinALDL spark display grid (user decision 2). 15 MAP columns × 5 chars + row label ≈ 84 chars: fits a normal terminal.

**Accessor** (`pkg/blm`): `Grid.Sum() [][]float64` — total of readings per cell (0 where empty), via the existing `floatGrid` helper. Generic; no ECM knowledge.

**Delta tracking** (TUI model, consumer-side): `knock_count` (byte, cumulative, wraps at 255) is differenced per parsed frame:

```go
sparkGrid *blm.Grid; knockPrev float64; hasKnockBase bool
// in accumulate, after the ParseOK gate:
cur := s.Sensors["knock_count"]
if m.hasKnockBase {
    if delta := math.Mod(cur-m.knockPrev+256, 256); delta > 0 {
        m.sparkGrid.Add(ft.RPM, ft.MapKPa, delta)
    }
}
m.knockPrev, m.hasKnockBase = cur, true
```

The first parsed frame only establishes the baseline (no delta — WinALDL counts knocks *during* the session, not the counter's absolute value). A cell's **Sum** is its knock-event count (a frame's delta may exceed 1); **Samples** is frames-with-knock.

**View** (`pkg/stream`): `gridHeat` generalizes to take the cell-values matrix — `gridHeat(g, values, ar, ac, minCount, prec, status, legend)` (internal; BLM/INT/O2 builders pass `g.Average()`, behavior identical). New builder:

```go
// SparkBody renders the knock-events grid: cells are total knocks counted in
// that RPM×MAP cell this session (delta of the cumulative KNOCK_CNT byte).
// Ungated like O2 — minCount is 1, cells never dim. knockCnt is the current
// frame's raw counter, shown in the status line.
func SparkBody(g *blm.Grid, ev FrameEvent, knockCnt float64) string
```

Status: `KNOCK_CNT <n>  RPM <r>  MAP <m> kPa`; active cell whenever the frame parses (same as O2); `prec=0`; legend `  knocks detected per cell this session; · = none`.

**Loop line**: `LoopStatus` gains a SPARK dot — `rec: BLM x INT x O2 x SPARK x` — with the same condition as O2 (`hasGood`; spark is ungated). Keeps the Phase 2 invariant that every grid tab's recording state is visible in the persistent chrome.

**TUI**: view enum becomes `viewSensors, viewBLM, viewINT, viewO2, viewSpark, viewFlags, viewCodes, viewRaw` (keys `1`–`8`, tab bar per user decision 3). `c` on the Spark tab reallocates `blm.NewSpark()` — the knock **baseline is kept** (clearing the table shouldn't manufacture a phantom delta). Body: `stream.SparkBody(m.sparkGrid, m.lastGood.FrameEvent, m.lastGood.Sensors["knock_count"])`.

## 5. Filename prompt (user decision 1)

A minimal modal line editor in the model — no new dependency:

```go
type promptTarget int // promptSave, promptRecord, promptCSV
type promptState struct { target promptTarget; buf, hint string } // hint: transient message (e.g. collision), cleared on next edit
prompt *promptState // nil when inactive
```

- **Open**: `s` always; `r`/`d` only when starting (stopping never prompts). `buf` pre-fills `goaldl_<ts>` (`time.Now().Format("20060102_150405")`).
- **While open**: printable runes append (digits included — tab switching and all other action keys are suspended); `backspace` deletes; `enter` confirms; `esc` cancels (notice `cancelled`); `ctrl+c` still quits. Snapshots keep flowing — grids/extrema/history keep accumulating and the body keeps rendering live behind the prompt.
- **Render**: the prompt replaces the footer's notice/legend segment: `  save as: goaldl_20260704_153012▌  .raw + enter confirm · esc cancel` (the extension hint matches the target; save shows `_BLM/_INT/_O2/_SPARK.txt`).
- **Confirm**: empty buffer → cancel. Extensions/suffixes are appended by the action (§1 `.raw`, §2 `.csv`, §6 the four `_<GRID>.txt`); the name is otherwise used verbatim — relative **and absolute** paths allowed. *(Implementation note: the TUI passes `saveGrids` dir `""` so the base is honoured verbatim; `filepath.Join(".", "/abs/…")` would silently relativize an absolute path — caught by the prompt tests.)*
- **Collision**: files are created `O_CREATE|O_EXCL` (save: all four checked first). If any target exists, notice `exists: <name> — edit the name` and the prompt **stays open** for editing. No silent overwrite.

## 6. Save — now four grids

`saveGrids` takes the user-edited base name instead of a timestamp and writes **four** files: `<base>_BLM.txt`, `_INT.txt`, `_O2.txt` (formats unchanged from Phase 2), plus `<base>_SPARK.txt`:

```go
func writeSparkFile(w io.Writer, g *blm.Grid) // Samples (frames with knock) + Knock counts (Sum, 0 dec)
```

`RenderInt("Samples (frames with knock)", g.Samples())` + `RenderFloat("Knock counts (delta of KNOCK_CNT)", g.Sum(), 0)`. No correction table (counts, not a trim). Empty grids still write (Phase 2 parity — headers + zeros, no silent skip).

## 7. Edge cases

- **Knock counter reset mid-session** (ECM power cycle): the wrap arithmetic reads a reset as one spurious positive delta (e.g. 200→0 ⇒ 56). Accepted — rare, self-limiting, and WinALDL has the same failure mode; noted in the `SparkBody` doc comment.
- **Drive fixture may contain zero knock events**: the end-to-end test asserts spark totals against an independent computation over the same frames — a legitimate result of 0 still verifies the pipeline (and crafted-frame unit tests cover nonzero deltas and the wrap).
- **`r` on replay / `space`/`+`/`-` on live / speed keys at `-speed 0`**: no-op with an explanatory notice (§1, §3). *(Refined post-implementation, user feedback 2026-07-04)*: these no-op warnings **self-expire after 3 s** (`noticeTTL`) via a `tea.Tick` guarded by a notice sequence number — a stale timer never clears a newer notice; action notices (saved/recording/stopped) remain until replaced.
- **Recording target write error**: session survives; recording detaches; footer notice (§1).
- **Prompt open when the stream ends**: still works — save/record/csv act on the accumulated state (record would capture nothing further; harmless).
- **Quit with recording/CSV active**: both closed cleanly after the Bubble Tea program exits.
- **Mid-stream capture start**: file begins at an arbitrary byte; decoder resync handles it (§1).
- **Pause during CSV logging**: no frames ⇒ no rows; `time_sec` stays on the data timeline either way.
- **Two saves in one second**: now user-visible in the prompt; `O_EXCL` turns a collision into an editable notice instead of an overwrite (improves the Phase 2 "noted, not guarded" stance for free).
- **Narrow terminal**: footer gains REC/CSV/speed segments and may soft-wrap; identifying info stays leftmost. Spark grid is 15 columns ≈ 84 chars — same class as the existing grids. Accepted.

## 8. Test plan (QA)

| Test | Where | Oracle |
|---|---|---|
| `Grid.Sum` totals per cell; `NewSpark` axes are 400–3600/400 × 30–100/5 | `pkg/blm` | arithmetic |
| `RecordSink`: discards+counts nothing when detached; counts bytes when attached; `Set` returns old target + count; write error detaches, sets sticky `Err`, session write keeps succeeding; concurrent `Write`/`Set` clean under `-race` | `pkg/stream` | crafted writers (incl. failing) |
| `ReplayProvider` pause: emissions stop while paused, resume continues, ctx cancel during pause returns promptly | `pkg/stream` | injectable now/sleep |
| `ReplayProvider` speed: `SetSpeed(2)` mid-run halves subsequent waits only (no retroactive jump — waits before the change unaffected); `Speed==0` ignores controls | `pkg/stream` | injectable now/sleep, recorded waits |
| Spark delta: baseline frame adds nothing; +2 then +0 then wrap 250→4 bins 2 and 10 into the right cells; sum vs samples distinguished (one frame with delta 3 ⇒ sum 3, samples 1) | `cmd/goaldl/tui_test.go` (model) | arithmetic on crafted frames |
| `SparkBody`: cells render **Sum** (two deltas 2+3 in one cell shows 5, not 2.5); never dimmed; active-cell highlight; status carries raw KNOCK_CNT | `pkg/stream` | golden-string assertions |
| `LoopStatus` SPARK dot in all four Phase 2 states (extends the existing table-driven test) | `pkg/stream` | table-driven |
| Prompt: digit keys append to the buffer (no tab switch) and `q` types rather than quits (only ctrl+c exits); esc cancels with no file; enter with edited base writes files under that name (temp dir); existing file ⇒ prompt stays open, no overwrite | `cmd/goaldl/tui_test.go` | model + temp-dir fs assertions |
| Keys: `r` on replay ⇒ notice, no prompt; `d` toggle over the drive fixture ⇒ CSV rows == ParseOK frame count, header matches `def.Parameters`, second `d` closes with row-count notice | `cmd/goaldl/tui_test.go` | `drive_4800.raw` + `frameCSV` format |
| Notice expiry: a no-op warning arms a timer and clears when it fires; a stale timer (older sequence number) does not wipe a newer notice | `cmd/goaldl/tui_test.go` | crafted `noticeExpireMsg` delivery |
| Save writes four files; `_SPARK.txt` = Samples + Knock counts, **no** correction table | `cmd/goaldl/tui_test.go` | temp dir + format assertions |
| End-to-end drive fixture: all 8 tabs render; spark total == independently computed KNOCK_CNT deltas over the same frames; BLM still 469 | `cmd/goaldl/tui_test.go` | `drive_4800.raw` + independent accumulation |
| Regression: decoder goldens byte-identical (no `-update`); `monitor`/`blm` untouched (`BLMBody`/`SensorTable` signatures unchanged; `gridHeat` change covered by existing BLM/INT/O2 view tests); `Snapshot`/`Session` API unchanged | existing suites | — |

## 8b. Post-implementation addition — grid explainers (user feedback, 2026-07-04)

Each grid tab must say **what the table means**, not just how to read its numbers. Every grid view (BLM / INT / O2 / Spark) renders an always-visible, dim "what this table means" block (4–6 lines, ≤78 chars/line) in place of the former one-line legend: what the table is, how to read it, and how to act on it (BLM: multiply base VE/fuel by avg/128; INT: trust sustained averages, cross-check against BLM; O2: ~0.45 V stoich, stuck cells = masked mixture offset; Spark: goal 0, repeating counts = pull timing/add fuel). The explainer text lives as constants in `pkg/stream/gridviews.go` next to the builders. **`monitor -blm` is unchanged**: `BLMBody` keeps its compact one-line legend (in-place streaming redraw); the dashboard uses a new `BLMBodyExplained` variant (both delegate to an unexported `blmBody`). Tested: each explainer present in its view (`TestGridExplainers` + per-tab TUI assertions); monitor's `BLMBody` asserted explainer-free; the spark never-dim assertion scoped to the grid rows (the explainer block is deliberately dim).

## 9. Out of scope (Phase 3)

Dash (big-number) view, config-file persistence, multi-ECM definitions (Phase 4); `serve` adapter (separate feature); Narrow/Avg10/StdDev grid modes and any decode-path filtering (permanent non-goals). `monitor` gains no spark view or runtime toggles (dashboard-only; its pre-declared `-o`/`-csv` flags remain the scripting idiom). No recording of replay sources.
