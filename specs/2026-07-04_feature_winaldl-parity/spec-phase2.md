<!-- SDA: v1.0 -->
# Spec: WinALDL Parity — Phase 2 (Tune)

Scope: plan.md Phase 2 steps 2.1–2.4. Phase 1 (Diagnose) is shipped. This phase turns the dashboard into a live tuning instrument: INT and O2 grids alongside BLM, in-TUI Save/Clear, and per-sensor Min/Max. Authorities unchanged: **A033.ads** for layout/bits, **`data/20250601_111156_LOG.txt`** for conversions, committed captures (`pkg/decoder/testdata/drive_4800.raw`) for accumulation tests.

**User decisions (2026-07-04)**:
1. **Tab order — group the grids**: `1 Sensors · 2 BLM · 3 INT · 4 O2 · 5 Flags · 6 Codes · 7 Raw`. The three tuning grids sit adjacent (matches WinALDL); Flags/Codes/Raw renumber to 5/6/7.
2. **Save writes all three grids at once**: one `s` press writes BLM + INT + O2 to three timestamped files (shared timestamp), from any tab.
3. **Always-on MIN/MAX columns** on the sensor tab (no mode cycling); `c` on that tab resets the extrema.
4. **Persistent loop-state chrome (added 2026-07-04)**: Open/Closed loop and per-grid recording state show as a fixed status line under the tab bar on **every** tab — because loop state governs whether the BLM/INT grids are accumulating at all, and therefore whether those tabs have any value at the moment. It is not buried in a single tab's body.

## 0. Architecture invariant (unchanged from Phase 1)

- **`stream.Snapshot` and `Session` do NOT change.** INT/O2 grids and per-sensor extrema are **consumer-side state** in the TUI model, accumulated from the existing Snapshot stream (`Sensors` + `FuelTrim`). This is deliberate per `architecture/session-api-layering`: the future `serve` adapter reconstructs the same grids/extrema from the same stream — no server-only fields on Snapshot.
- **`pkg/blm` does NOT change.** INT and O2 are the same `blm.Grid` (RPM×MAP accumulator) on the default axes, with different value sources and gating. `RenderInt`/`RenderFloat` already take precision, so they render the O2 grid (3 decimals) and the save files as-is.
- **`pkg/ecm` does NOT change.** `integrator` (offset 9) and `oxygen_sensor` (offset 10, mV) are already parsed parameters. No new frame-layout knowledge is introduced anywhere outside `pkg/ecm`.
- **Decode path untouched** → decoder goldens stay byte-identical; no `-update`.

Net: Phase 2 is presentation + consumer-side accumulation only. Changes land in `pkg/stream` (view builders) and `cmd/goaldl/tui.go` (model + wiring), plus tests.

## 1. Grid value sources and gating (steps 2.1, 2.2)

All three grids bin on the same axes (`blm.NewDefault()` → RPM 400..6400/400, MAP 20..100/10) using the current frame's `FuelTrim.RPM` and `FuelTrim.MapKPa` (always populated from the frame, independent of gating).

| Grid | Value binned | Gate (accumulate when…) | Neutral / correction | Live cell precision |
|------|--------------|-------------------------|----------------------|---------------------|
| **BLM** (existing) | `Sensors["blm"]` (byte, offset 18) | `FuelTrim.Recordable()` = ClosedLoop **and** BLMEnabled | 128; correction = avg/128 | 0 dec |
| **INT** (new) | `Sensors["integrator"]` (byte, offset 9) | `ParseOK` **and** `FuelTrim.ClosedLoop` | 128; correction = avg/128 | 0 dec |
| **O2** (new) | `Sensors["oxygen_sensor"] / 1000` → volts (offset 10) | `ParseOK` only (ungated — populates immediately) | none (voltage; no correction) | 2 dec live (3 dec in status + saved file) |

Notes:
- INT gates on **ClosedLoop only** (the integrator runs in closed loop; block-learn-enable is a BLM-specific gate). This is intentionally distinct from BLM's dual gate.
- O2 is ungated: any `ParseOK` frame adds. Its live cells use `minCount = 1` (never dimmed — WinALDL shows O2 immediately). INT reuses the BLM `-min` threshold for dimming (same "cell not yet trusted" idea).
- Accumulation reads values only from `Snapshot.Sensors`/`FuelTrim` — the TUI never touches frame bytes for grid values (keeps offsets in `pkg/ecm`). Gate on `ParseOK` so an empty `Sensors` map (parse failure) never bins a spurious 0.

## 2. View builders — `pkg/stream`

Refactor the shared heatmap out of `BLMBody` into an unexported helper, then add INT/O2 builders. `BLMBody`'s signature is preserved (the `monitor -blm` live `BLMView` depends on it).

```go
// gridHeat renders a status line + the RPM×MAP heatmap (active cell reverse-
// video, cells below minCount dimmed, "·" for empty), rounding each cell to
// prec decimals. Shared by the BLM, INT, and O2 grid views.
func gridHeat(g *blm.Grid, ar, ac, minCount, prec int, status, legend string) string
```

- `BLMBody(g, ev, minCount)` — unchanged behavior; now computes its status/active-cell then calls `gridHeat(..., prec=0, ...)`.
- `INTBody(g, ev, minCount, intVal)` (new) — `intVal` is the current frame's integrator (from `Snapshot.Sensors["integrator"]`, passed by the consumer — avoids adding a field to `ecm.FuelTrim`); active cell from `FuelTrim` when `ClosedLoop`; status `CLOSED LOOP  RPM … MAP … kPa  INT n` or `OPEN LOOP — integrator frozen`; `prec=0`; legend `target 128:  >128 adding fuel (lean), <128 removing (rich)`. *(Implemented signature: the value is threaded in rather than re-derived, since `FuelTrim` carries only BLM.)*
- `O2Body(g, ev, o2Volts)` (new; `minCount` fixed at 1 internally) — `o2Volts` from `Snapshot.Sensors["oxygen_sensor"]/1000`, passed by the consumer; active cell whenever the frame parsed; status `O2 x.xxx V  RPM … MAP …` (3-decimal current reading); grid cells `prec=2` — a 3-decimal cell fills the whole 5-wide column so columns collide; 2 decimals keeps a leading-space gutter and lets the dense heatmap breathe. The **saved** O2 file keeps full 3-decimal precision (tab-separated, no collision). Legend `volts; higher = richer exhaust; · = no data`.

These live in a new `pkg/stream/gridviews.go` (or appended to `statusviews.go`), same inline-ANSI idiom as `BLMBody`.

**Persistent loop-state line (decision 4)**. Loop state is promoted from the BLM tab's body into fixed chrome rendered on every tab. A pure builder produces it so it stays testable and reusable (a `serve` adapter would surface the same fields):

```go
// LoopStatus renders the one-line, always-visible recording-state badge:
// the loop mode plus a per-grid ● accumulating / ○ frozen indicator, so the
// operator can tell from any tab whether the grid they're on is live.
func LoopStatus(ft ecm.FuelTrim, hasGood bool) string
```

Content, driven entirely by `FuelTrim` (already on every Snapshot — no new data):

| Condition | Badge | BLM | INT | O2 |
|---|---|---|---|---|
| ClosedLoop && BLMEnabled | `CLOSED LOOP` (green) | ● | ● | ● |
| ClosedLoop && !BLMEnabled | `CLOSED LOOP` (green) | ○ | ● | ● |
| !ClosedLoop | `OPEN LOOP` (amber) | ○ | ○ | ● |
| no good frame yet | `LOOP —` (dim) | ○ | ○ | ○ |

Rendered e.g. `CLOSED LOOP   rec: BLM ● INT ● O2 ●` / `OPEN LOOP   rec: BLM ○ INT ○ O2 ●  (grids frozen)`. `●` = accumulating this frame, `○` = frozen (values held, not learning). O2's dot is ● whenever a frame parses (ungated). The color lives in `cmd/goaldl/tui.go` (lipgloss, like the existing `beatOK`/`beatBad`); `LoopStatus` returns the plain text + inline markers, and the TUI wraps the badge word in the loop color.

`BLMBody` is **left unchanged** — it is shared with the non-tabbed `monitor -blm` streaming view, which has no persistent chrome and needs the OPEN/CLOSED word in its body. `INTBody` (TUI-only) mirrors `BLMBody`'s status shape for consistency on the adjacent tab. The resulting minor redundancy on the BLM/INT tabs (loop word in both the chrome line and the body's detail line) is accepted and reinforcing; the real win is that Sensors/O2/Flags/Codes/Raw — where loop state was previously invisible — now carry it too.

**Sensor table — always-on MIN/MAX (step 2.4)**. Extend the table to a 6-column layout for the dashboard while leaving `monitor`'s 4-column path untouched:

```go
// Row gains Min, Max string (blank when not applicable / no data yet).
type Row struct { Sensor, Raw, Value, Min, Max, Alt string }

// SensorTableExtrema renders SENSOR · RAW · VALUE · MIN · MAX · ALT, with
// per-parameter extrema pulled from mins/maxs (keyed by parameter Name).
// mins==nil → falls back to the existing 4-column SensorTable (monitor path).
func SensorTableExtrema(ev FrameEvent, def *ecm.Definition, mins, maxs map[string]float64) string
```

- Columns: `SENSOR · RAW · VALUE · MIN · MAX · ALT`. MIN/MAX sit next to VALUE (grouping the three magnitudes); ALT (dual-unit of the *latest* value only) stays last.
- MIN/MAX format with the parameter's **primary** unit via the existing `formatNum`. Blank ("—") until the first `ParseOK` frame after a reset.
- `SensorTable` (4-col) is unchanged so `monitor` and its tests don't move.

## 3. TUI — `cmd/goaldl/tui.go`

**Tabs / view enum** reorder to group grids:

```go
const ( viewSensors view = iota; viewBLM; viewINT; viewO2; viewFlags; viewCodes; viewRaw; viewCount )
```
Labels: `1 Sensors · 2 BLM · 3 INT · 4 O2 · 5 Flags · 6 Codes · 7 Raw`. Digit keys `1`–`7` select directly; `tab`/`←`/`→`/`h`/`l` cycle all 7.

**Model additions** (consumer-side state):
```go
intGrid, o2Grid *blm.Grid          // built with blm.NewDefault() alongside grid (BLM)
mins, maxs      map[string]float64 // per-parameter extrema since last reset
hasExtrema      bool
notice          string             // transient footer message after save/clear
```

**Update — accumulation** (in the `snapshotMsg` case, after the existing BLM add). All gated on `s.ParseOK`:
- INT: if `s.FuelTrim.ClosedLoop` → `m.intGrid.Add(rpm, map, s.Sensors["integrator"])`.
- O2: `m.o2Grid.Add(rpm, map, s.Sensors["oxygen_sensor"]/1000.0)`.
- Extrema: for each parsed `(name, v)`, update `mins[name]=min`, `maxs[name]=max`; set `hasExtrema`.

**Update — keys**:
- `s` (any tab): `saveGrids(...)`; set `m.notice = "saved BLM/INT/O2 → goaldl_<ts>_*.txt"`.
- `c` (context-sensitive):
  - `viewBLM/viewINT/viewO2` → clear the **current** grid (reallocate `blm.NewDefault()`); notice `"cleared <NAME> grid"`. (Clear is per-table, WinALDL-style — only the viewed grid.)
  - `viewSensors` → reset extrema (`mins/maxs = map{}`, `hasExtrema=false`); notice `"reset min/max"`.
  - other tabs → no-op.

**View — bodies**:
```go
case m.active == viewSensors: body = stream.SensorTableExtrema(m.lastGood.FrameEvent, m.def, m.mins, m.maxs)
case m.active == viewBLM:      body = stream.BLMBody(m.grid,    m.lastGood.FrameEvent, m.minSamples)
case m.active == viewINT:      body = stream.INTBody(m.intGrid, m.lastGood.FrameEvent, m.minSamples)
case m.active == viewO2:       body = stream.O2Body(m.o2Grid,   m.lastGood.FrameEvent, 1)
case m.active == viewFlags:    body = stream.FlagsBody(m.lastGood.Flags)
case m.active == viewCodes:    body = stream.CodesBody(m.lastGood.Codes)
```
Footer gains the notice (when set) and the extended key legend: `1-7/tab switch · s save · c clear · q quit`.

**Persistent loop-state line (decision 4)**. Rendered as fixed chrome between the tab bar and the body, on every tab (independent of `m.active`). Derived from `m.lastGood.FuelTrim` (the latest *parseable* frame) so a single bad frame doesn't flicker the badge — consistent with the lastGood gating used for all decoded views:

```go
header := tabBar + "\n" + m.loopStatusLine()   // loopStatusLine wraps stream.LoopStatus + loop color
// body follows on its own line as today
```
`loopStatusLine` calls `stream.LoopStatus(m.lastGood.FuelTrim, m.hasGood)` and colors the badge word green (CLOSED) / amber (OPEN) / dim (`LOOP —`, before the first good frame), reusing the lipgloss style pattern of `beatOK`/`beatBad`. It sits above the body so it never scrolls away with tab content.

## 4. Save — `cmd/goaldl/tui.go` (step 2.3)

```go
// saveGrids writes BLM, INT, and O2 to three timestamped files in the working
// directory, sharing one timestamp. Returns the base name for the notice.
func saveGrids(dir string, ts time.Time, blmG, intG, o2G *blm.Grid, minSamples int) (base string, err error)
// core, pure and testable — one grid to a writer (implemented names):
func writeTrimGridFile(w io.Writer, g *blm.Grid, name string, minSamples int) // Samples + Wide Average + Correction (BLM/INT)
func writeO2File(w io.Writer, g *blm.Grid)                                     // Samples + Wide Average (3 dec, volts)
```

- Filenames: `goaldl_YYYYMMDD_HHMMSS_BLM.txt`, `…_INT.txt`, `…_O2.txt` (`ts.Format("20060102_150405")`).
- **BLM & INT** files: `Grid.RenderInt("Samples", …)` + `Grid.RenderFloat("Wide Average BLM/INT (target 128; >128 lean, <128 rich)", Average(), 1)` + a `Correction factor = avg/128 (cells with <N samples held at 1.000)` header + `RenderFloat("", CorrectionAtLeast(min), 3)`. Byte-for-byte the same table shape as the `blm` command and `data/20250601_162123_BLM.txt`.
- **O2** file: `RenderInt("Samples", …)` + `RenderFloat("Wide Average O2 (volts)", Average(), 3)`. No correction (it is a voltage, not a trim multiplier).
- Written to the current working directory. Empty grids still write (headers + zeros / 1.000) — parity with WinALDL's unconditional Save; no silent skip.

## 5. Edge cases

- **No closed-loop frames** (cold/WOT capture): BLM and INT grids stay empty; O2 still populates (ungated). Grid views render the empty grid + their "not recording / frozen" status line.
- **`ParseOK` never true** (garbage capture): all three grids empty, extrema all "—", O2 empty (needs a parsed O2); Raw view still streams (Phase 1 behavior).
- **Save with empty grids**: files are written with header rows and no samples — intentional (see §4).
- **`c` on a non-grid, non-sensor tab** (Flags/Codes/Raw): no-op (notice unchanged).
- **`s` from any tab**: always saves all three grids (global action) — convenient at the car regardless of the active tab.
- **Narrow terminal**: the 6-column sensor table may soft-wrap; MIN/MAX/ALT are the rightmost columns, so the identifying SENSOR/RAW/VALUE stay left. Grid heatmaps are fixed width (unchanged). Accepted.
- **Timestamp collision** (two saves within one second): second-resolution names would overwrite; acceptable and rare — noted, not guarded.
- **O2 value scaling**: `oxygen_sensor` is parsed in mV (factor 4.44); the grid bins volts (`/1000`) so the heatmap and file read like WinALDL's 3-decimal volts.
- **Loop line before the first good frame**: `hasGood == false` → badge renders dim `LOOP —` with all recording dots `○` (nothing is accumulating yet). After a good frame it reflects real state and holds the last good value through subsequent bad frames and after the stream ends.

## 6. Test plan (QA)

| Test | Where | Oracle |
|---|---|---|
| INT grid: closed-loop frames bin the integrator; open-loop frames skipped; a known cell's Wide Average matches a hand-accumulated value over the drive fixture | `pkg/stream` (or `cmd` model test) | `drive_4800.raw` + independent accumulation |
| O2 grid: ungated — populates on every `ParseOK` frame (count == parsed-frame count); binned value == `oxygen_sensor/1000` | `pkg/stream` | `drive_4800.raw` |
| `gridHeat` precision: O2 cells render 3 decimals, BLM/INT 0; active cell reverse-video; empty "·"; dim below minCount | `pkg/stream` | golden-string assertions |
| `SensorTableExtrema`: 6 columns; MIN/MAX equal running extrema over two crafted frames; ALT still present; nil extrema → 4-col `SensorTable` | `pkg/stream` | arithmetic |
| Model accumulates all three grids + extrema over the drive fixture; counts and a sample cell match | `cmd/goaldl/tui_test.go` | `drive_4800.raw` |
| Keys: `1`–`7` select the right view; `c` on a grid tab clears only that grid (others retain samples); `c` on Sensors resets extrema; `s` sets the notice | `cmd/goaldl/tui_test.go` | model assertions |
| `LoopStatus`: the four states (closed+enabled / closed+disabled / open / no-good-frame) each yield the right badge word and BLM/INT/O2 ●/○ pattern; O2 dot is ● whenever closed **or** open (ungated) | `pkg/stream` | table-driven |
| Loop line is present on every tab and holds `lastGood` across a following bad frame (no flicker) | `cmd/goaldl/tui_test.go` | crafted open→closed→bad sequence |
| Save format: INT/BLM files contain Samples + Wide Average + Correction; O2 file has 3-decimal averages and **no** correction line; a cell average matches the grid | `cmd/goaldl/tui_test.go` | `writeTrimGridFile`/`writeO2File` via `saveGrids` to a temp dir |
| Regression: decoder goldens byte-identical; Snapshot/session tests unchanged (no API change); `monitor` 4-col table unchanged | existing suites | — |

## 7. Out of scope (Phase 2)

Spark-counts grid, recording toggle, replay pause/speed keys, CSV toggle (Phase 3); Dash view, config persistence, multi-ECM (Phase 4); Narrow/Avg10/StdDev grid modes and any decode-path filtering (permanent non-goals). `monitor` gains no INT/O2 view this phase (dashboard-only).
