<!-- SDA: v1.0 -->
# Spec: TUI UX Pass — Phase C (Session safety + unified outputs)

**Scope**: plan.md Phase C. Two connected bodies of work:

1. **C.0 Unify session outputs** (user direction, 2026-07-04; models WinALDL's LOG Data checklist, `docs/winaldl/log.gif`). Replace today's three ad-hoc output actions (`r` raw-record, `d` CSV log, `s` save-grids) with **two operations that each pick their formats from a checklist at trigger time**: **Save Buffer** (retroactive, dumps a bounded in-memory ring) and **Log** (forward, crash-tolerant streaming). Backed by a new bounded **decoded-frame ring buffer** with a **% full** indicator.
2. **C.1–C.4 Session safety** (F5, F14): dirty tracking, a quit guard, a clear-undo, and an exit summary + a formalized notice-lifecycle rule.

All of it is **consumer/presentation-only** — `cmd/goaldl` plus reuse of the *existing* `stream.RecordSink`, `blm.Grid` save methods, and the `frameCSV` writer. **No `Session`/`Snapshot`/`pkg/ecm`/`pkg/decoder`/`pkg/blm`/`go.mod` change; forbidden-seam diff stays empty; goldens byte-identical.**

**User decisions carried in (2026-07-04)**:
- Format is chosen **per invocation** (checklist opens when you press the key) — not a persistent profile.
- Rename: Save → **Save Buffer**; Record → **Log** (WinALDL's word). Save Buffer offers **no RAW** (unreconstructable from decoded frames); RAW lives only on **Log**, off by default.
- The decoded-frame buffer is a **bounded ring** (oldest dropped) with a **% full** indicator.

**Spec decisions taken here** (the plan's open items #0–#6 that touch Phase C):
- **Ring capacity**: `frameBufCap = 3600` frames (≈ 60 min at the ~1.2 s cadence; ≈ 1 MB at the compact per-frame record below). A const, not a flag.
- **Flags/Codes file outputs**: **deferred** from this slice (see Non-goals) — the two decoded formats shipped are **Sensor CSV** (reuses `frameCSV`) and the **grid dumps** (reuse `saveGrids`). Checklist is built to extend.
- **US/Metric radio**: **deferred** — goaldl's CSV writes each parameter's native unit today; an export unit toggle is a separate refinement.
- **C.3 clear guard**: **one-slot undo** (`u`), not double-tap confirm — grids are pointers, so undo is a retained pointer restore; cheaper in the common case.
- **F18 (single-grid save)** and **F17 (show destination dir)** fall out of this design for free (see C.0) and are folded in.

---

## C.0 — Unified outputs: Save Buffer + Log

### Current behavior
Three independent actions, three formats, three code paths:
- `s` (`tui.go:418`) → filename prompt → `saveGrids("", base, …)` writes four `<base>_{BLM,INT,O2,SPARK}.txt` (all grids, always).
- `r` (`tui.go:422`, `toggleRecording`) → filename prompt → attaches `recSink` to a `<base>.raw`, streams raw bytes forward; toggling stops it.
- `d` (`tui.go:425`, `toggleCSV`) → filename prompt → opens a `frameCSV` on `<base>.csv`, writes each ParseOK frame forward (`tui.go:467-471`); toggling stops it.

The model has no history: it keeps `latest`/`lastGood` (one frame each) + the live-accumulated grids. So a decoded CSV can only be produced *going forward* — there is no way to save the frames you already watched.

### Design

**Decoded-frame ring buffer (new, `cmd/goaldl/framebuf.go`).** A fixed-capacity ring of compact per-frame records, appended for **every** frame in `snapshotMsg` (parseable or not — a RAW-equivalent decoded view; ParseOK is recorded per-row so the CSV can mirror `frameCSV`'s ParseOK-only output):

```go
// bufFrame is the compact projection the ring retains — the fields the CSV
// export needs, plus the aligned frame bytes so a future export can re-parse
// under a different ECM layout. Values are stored in def.Parameters order (no
// per-frame map) to bound memory deterministically.
type bufFrame struct {
    data       []byte    // 20 aligned frame bytes (copied; snapshot data is reused)
    elapsedSec float64
    byteOffset int64
    parseOK    bool
    promOK     bool
    vals       []float64 // parsed sensor values, def.Parameters order (nil if !parseOK)
}

type frameBuf struct {
    cap   int
    ring  []bufFrame
    head  int  // next write index
    n     int  // live count (≤ cap)
    total int  // frames ever pushed (high-water; drives % full once total<cap)
}

func (b *frameBuf) push(f bufFrame)        // O(1); overwrites oldest when full
func (b *frameBuf) frames() []bufFrame     // oldest→newest snapshot for export
func (b *frameBuf) fillPct() int           // min(100, n*100/cap)
```

`const frameBufCap = 3600`. Memory: `data` (20 B) + `vals` (~20 float64 = 160 B) + header ≈ ~200 B/frame → ~0.7 MB full. The `vals` slice is built once at push time from `def.Parameters` (same iteration `frameCSV.Write` already does), so the live `Sensors` map isn't retained — no map-lifetime coupling, no unbounded growth.

**CSV export from the buffer.** Factor `frameCSV` so both the live path and the buffer path share one row writer. Add:
```go
func (c *frameCSV) WriteRow(f bufFrame) // writes only when f.parseOK (parity with live ParseOK-only rows)
```
and have the existing `Write(tSec, off, promOK, map)` build the ordered `vals` and delegate — or, simpler, keep `Write` for the live path and give `WriteRow` its own body reading `f.vals` by index. Either way the header and float formatting (`%.2f`) are unchanged, so a buffer-dumped CSV is byte-identical to what the live `d` toggle produced over the same frames.

**The output picker modal (new, replaces the plain filename prompt for `s`/`r`).** A single modal carrying a checklist **and** the filename field, confirmed once:

```go
type outputOp int
const ( opSaveBuffer outputOp = iota; opLog )

type fmtItem struct {
    id    string // "csv", "blm", "int", "o2", "spark", "raw"
    label string // "Sensor CSV", "BLM grid", …, "RAW bytes"
    on    bool
}

type outputPicker struct {
    op     outputOp
    items  []fmtItem
    cursor int    // 0..len(items)+1 → dir field, then name field
    dir    string // destination directory, pre-filled with the working dir — editable
    name   string // base name, pre-filled defaultBase()
    hint   string // collision / error, cleared on next edit
}
```

- **Save Buffer** items: `Sensor CSV` (on), `BLM grid` (on), `INT grid` (on), `O2 grid` (on), `SPARK grid` (on). **No RAW.**
- **Log** items (**same set as Save Buffer plus RAW**, user follow-on 2026-07-05): `RAW bytes` (**off**, disabled-not-hidden on replay), `Sensor CSV` (on), `BLM grid` (on), `INT grid` (on), `O2 grid` (on), `SPARK grid` (on). RAW/CSV stream forward; the **grids are session aggregates**, so a Log writes them **continuously (every frame), atomically (temp + fsync + rename)** so the last complete tables survive a crash — the write is not deferred to stop. Rationale: someone logging a drive wants its BLM/INT/O2/SPARK tables too, and must not lose 45 min of learning to a crash.

Picker keys (while `m.picker != nil`, mirroring the existing prompt capture at `tui.go:380`): `↑`/`↓` move the cursor across the items and the two path fields; `space` toggles the item under the cursor (no-op on a field row); printable runes / backspace edit whichever **path field** (dir or name) is focused (so digits don't fight the checklist); `enter` confirms; `esc` cancels; `ctrl+c` quits. **The destination directory and the filename are both editable** (user follow-on, 2026-07-05): the `dir` field is pre-filled with the working directory, the `name` field with `defaultBase()`; on confirm they are joined (`filepath.Join(dir, name)`), and a path typed into either is still honoured. This supersedes the earlier read-only `dir:` display — F17 is now met by an editable field, not just a shown one. The rendered modal shows each item as `[x]`/`[ ]`, then `dir: …` and `name: …` rows with a `▌` caret on the focused field.

**Confirm — Save Buffer** (`confirmSaveBuffer`): with the selected set and trimmed base,
1. Pre-check every target path for existence (exclusive-create semantics as `saveGrids` already does); any collision → `hint = "exists — edit the name"`, keep the modal open (no file written).
2. Write the selected grids via `saveGrids`-style per-file writers (refactor `saveGrids` to take the selected subset — see below), and, if `Sensor CSV` is selected, open a `frameCSV` and replay `m.buf.frames()` through `WriteRow`, then close it.
3. Record each written file in `m.written` (name + rows/bytes) for the exit summary, clear the grid-dirty flag if grids were written, set a persistent notice `saved <n> file(s) → <base>_*`, close the modal.

`saveGrids` today writes a fixed four. Refactor to `saveGrids(dir, base string, sel []gridSel, minSamples int)` where `gridSel{suffix, grid}` — the caller passes only the selected grids. This directly delivers **F18 (single-grid save)**: unchecking three boxes saves one grid.

**Confirm — Log** (`confirmLog`): with the selected set and base,
1. Pre-check **every** target path — `<base>.raw`, `<base>.csv`, and each selected `<base>_<GRID>.txt` — for existence; collision → hint, keep open (no partial start).
2. For `RAW`: `os.OpenFile(name, O_CREATE|O_EXCL|O_WRONLY)` → `recSink.Set(f)`. For `Sensor CSV`: `newFrameCSV(name, def)`. Store both handles (streaming).
3. Remember the selected grid ids in `m.logGridIDs` + `m.logBase`, and write them once now (`rewriteLogGrids`) so the files exist immediately; notice `logging → <files> · N grid(s) (live)`, close the modal.

Forward streaming reuses the **existing** `snapshotMsg` hooks (`m.csvLog`/`m.recFile` blocks, unchanged) and adds a per-frame `rewriteLogGrids()` call. **Crash durability:** `rewriteLogGrids` rewrites each selected grid file every frame via `atomicWriteFile` (write to `<path>.tmp`, `fsync`, `os.Rename` over the target) — a crash mid-write leaves the previous complete table intact, never a torn one; the ~4 tiny writes/frame at the ~1.2 s cadence are negligible. Fail-soft like `RecordSink`: a rewrite error detaches grid logging with a notice, never kills the session. `stopLog` does a final flush + records the files in `m.written`; `closeOutputs` does a final flush so a clean quit captures the last frame. `toggleLog`/`logActive` treat pending grids as an active Log even with no stream open (grids-only Log is valid).

**Key bindings.**
- `s` → open the Save Buffer picker.
- `l` → **Log**: if a Log stream is open, stop it (close `recFile`+`csvLog`, notice `stopped log (…)`); else open the Log picker. (User chose `l` for Log, 2026-07-05; the vim `l`/`h` cycle-right/left aliases were dropped — `tab`/`shift+tab`/`←`/`→` still cycle.)
- `d` is **removed** from the key switch and the legend.
- `u` → undo the last clear (C.3).

### Edge cases
- **Empty selection**: confirm with no items checked → notice `nothing selected`, modal stays open (or closes on a second esc). No file written.
- **Buffer not yet full**: `fillPct()` < 100; Save Buffer dumps `n` frames (the whole session so far). A brand-new session (0 frames) → Save Buffer writes header-only CSV + empty grids, same as today's empty save.
- **Ring wrapped**: Save Buffer dumps the most recent `frameBufCap` frames in order; the notice/exit-summary reports the row count so the truncation is visible (never silent).
- **Log on replay**: RAW needs a live serial stream to tee (`recSink == nil` on replay). Rather than hide it, the RAW item is **shown disabled** (dimmed `RAW bytes (live only)`, user follow-on 2026-07-05) so the capability stays discoverable; `space` on it is a no-op that hints why, and `selected()` excludes disabled items. Sensor CSV Log still works on replay.
- **Collision mid-set**: pre-checking *all* targets before writing any preserves `saveGrids`' current all-or-nothing guarantee — a name clash on one file never leaves a partial set.

---

## C.1 — Dirty tracking

**Model fields.** `dirtyGrids bool` — set true whenever `accumulate` adds to any grid (i.e. `grid`/`intGrid`/`o2Grid`/`sparkGrid` received a value since the last grid-inclusive Save Buffer). Cleared when a Save Buffer writes grids. `logging bool` — any Log stream currently open (`recFile != nil || csvLog != nil`).

**The ring buffer does NOT count as dirty.** It is ephemeral scratch by design (bounded, self-overwriting); treating it as unsaved would make the quit guard fire on every session. "Unsaved" means the **grids** (the session's tuning product, per F5) or an **open Log** (a stream the user presumably wants to finalize). Documented in code.

---

## C.2 — Quit guard (F5)

**Behavior.** On `q` (not `ctrl+c`):
```
if m.logging || m.dirtyGrids {
    if !m.quitArmed { arm; notice; return (no quit) }
    // second q → fall through to quit
}
```
- First `q` when dirty/logging → set `m.quitArmed = true`, persistent notice: `unsaved grids · logging active — q again to quit, s to save`. Do **not** quit.
- Second `q` (while armed) → `cancel()` + `tea.Quit`.
- Any other key (or a successful Save Buffer) **disarms** (`quitArmed = false`, notice cleared) so a stray earlier `q` can't cause a later single-`q` to quit unexpectedly.
- `ctrl+c` is the unconditional escape hatch (already immediate at `tui.go:386`), unchanged.
- Clean state (no logging, grids saved/empty) → `q` quits immediately, as today.

`quitArmed` is a pure model flag; tested by feeding `q` twice and asserting no `tea.Quit` on the first, `tea.Quit` on the second, and disarm on an intervening key.

---

## C.3 — Clear undo (F5)

**One-slot undo.** `clear()` (`tui.go:570`) already swaps a grid pointer for a fresh one (or resets extrema). Retain the previous value so `u` restores it:

- Fields: `undoGrid *blm.Grid`, `undoView view`, plus `undoMins/undoMaxs` for the sensor-tab extrema case.
- `clear()`: before replacing, stash the outgoing pointer/maps and `undoView = m.active`; notice gains a suffix — `cleared BLM grid (u to undo)`.
- New `u` handler: if `undoView` is set, restore the stashed grid/extrema to that view's field, notice `restored BLM grid`, consume the slot (`undoView = -1`). Only the **most recent** clear is undoable; a second clear overwrites the slot.
- Undo restoring a grid also restores `dirtyGrids` to true (the data is live again).

Undo is one retained pointer — no deep copy, since `clear()` replaces (never mutates) the grid. Documented alongside the existing "clear keeps the knock baseline" rule.

---

## C.4 — Exit summary + notice lifecycle (F14)

**Exit summary.** After `tea.NewProgram(...).Run()` returns and the alt-screen tears down (`cmdTUI`, after `closeOutputs`), print a one-block summary to stderr (survives teardown, scriptable), gated on having processed ≥1 frame:
```
goaldl session — live:/dev/cu.usbserial-10
  frames: 612 ok / 3 bad · buffer high-water 615 (100%)
  wrote:  goaldl_20260705_… _BLM.txt (37 cells)
          goaldl_20260705_… .csv (612 rows)
```
Files come from `m.written` (appended by every successful Save Buffer / Log-stop with a rows/bytes detail). No files → the `wrote:` block is omitted. The existing `fatalErr` reprint (A.1) prints first when set; the summary is additive and independent.

**Notice-lifecycle rule (F14), made deliberate and documented.** Two notice classes already exist; formalize the distinction in code (a doc comment on `setNotice`/`warn`) rather than change behavior:
- `warn(text)` — **transient**: no-op / rejected-action feedback (e.g. `pause/speed are replay-only`), self-expires after `noticeTTL`.
- `setNotice(text)` — **persistent**: a completed action's confirmation (saved, cleared, logging started), holds until the next action replaces it.

This is the current behavior; C.4 just pins it as the intended contract so future notices pick the right helper. (F16's heartbeat glyph and other learnability items stay in Phase E.)

---

## Non-goals (Phase C)
- **Flags/Codes as file outputs** — deferred. WinALDL's LOG dialog offers them; this slice ships Sensor CSV + grids (writers that already exist) and leaves the checklist extensible. They remain live-viewable on their tabs, and their raw bytes are recoverable from a RAW Log. (Follow-up: two time-series writers + two checklist items.)
- **US/Metric export radio** — deferred (export writes native units, as today).
- **Persistent format profile / config file** — out of scope (no config layer yet); the checklist is per-invocation.
- **Replay position/seek, port discovery, byte diagnostics** — Phase D.
- **`?` overlay, context footer, rec→learn terminology beyond the `r`-labels-as-"log" change, PROM-gated extrema** — Phase E.
- **No provider/core change**: `Session`/`Snapshot`/`ReplayProvider`/`SerialProvider`/`pkg/ecm`/`pkg/decoder`/`pkg/blm` untouched. `RecordSink` is reused as-is. The ring buffer, picker, guards, and summary are all `cmd/goaldl`.
- **No decode-path change**; decoder goldens stay byte-identical; `blm` still 469 over the drive fixture.

## Files changed (planned)
| File | Change |
|---|---|
| `cmd/goaldl/framebuf.go` (new) | `bufFrame`, `frameBuf` ring (`push`/`frames`/`fillPct`), `frameBufCap` |
| `cmd/goaldl/tui.go` | ring push in `snapshotMsg`; `outputPicker` modal (state + key handling + render) replacing the plain prompt for `s`/`r`; `confirmSaveBuffer`/`confirmLog`; `r`→Log toggle, remove `d`; `u` undo; `dirtyGrids`/`logging`/`quitArmed`/`undo*`/`written`/`buf` fields; quit guard in the `q` case; buffer `% full` + `LOG`/`REC` chrome in the footer; `keyLegend` updated; `defaultBase` reused; exit summary in `cmdTUI` |
| `cmd/goaldl/csv.go` | `frameCSV.WriteRow(bufFrame)` sharing the row body with `Write`; header/format unchanged |
| `cmd/goaldl/capture.go` (or where `saveGrids` lives) | `saveGrids` takes a selected-grid subset (`[]gridSel`); callers pass the picker selection |
| `cmd/goaldl/tui_test.go` | new tests below |
| `cmd/goaldl/framebuf_test.go` (new) | ring semantics |

## Test plan
Model-level, `.Update(msg)` idiom, real `frameCSV`/`saveGrids`/`blm.Grid`; filesystem writes go to `t.TempDir()`.

1. **`TestFrameBuf`** (unit): push `cap+50` frames → `n == cap`, `frames()` returns the last `cap` in oldest→newest order, `fillPct()` 100; partial fill → `n == pushed`, correct pct; empty → 0 frames, 0%.
2. **`TestSaveBufferCSV`**: drive the model over `drive_4800.raw` (real `Session`/`ReplayProvider`, as `TestTUIDriveFixtureEndToEnd`), open the Save Buffer picker, select only `Sensor CSV`, confirm to a temp name → the CSV has the `frameCSV` header and one row per **ParseOK** buffered frame, values byte-identical to a live `frameCSV` over the same frames. Assert the retroactive property: the model **never had a live CSV open** yet produced the rows.
3. **`TestSaveBufferGridSubset`** (F18): select only `BLM grid` → exactly `<base>_BLM.txt` written, the other three absent; content matches today's `saveGrids` BLM file. Collision on the BLM name → hint set, modal open, no file written.
4. **`TestLogForward`**: open the Log picker on a live-like model, select `RAW`+`Sensor CSV`, confirm → `recFile`/`csvLog` set, `logging` true; feed frames → CSV rows + `recSink` bytes grow; `r` again → both closed, `logging` false, `written` has both with row/byte details. RAW item hidden when `recSink == nil` (replay).
5. **`TestQuitGuard`** (F5): dirty model + `q` → no `tea.Quit`, `quitArmed`, notice present; second `q` → `tea.Quit`; `q` then a tab key then `q` → no quit (disarmed); clean model + `q` → immediate `tea.Quit`; `ctrl+c` on a dirty model → immediate quit.
6. **`TestClearUndo`** (F5): accumulate BLM, `c` on BLM → grid empty, notice has `u to undo`; `u` → grid restored (cell count back), `dirtyGrids` true; second `c` then `u` restores the second, first is gone (one slot).
7. **`TestExitSummary`**: after a run with a Save Buffer, `sessionSummary()` (pure, over `m.written`+counts) contains frames ok/bad, buffer high-water, and each written file with its detail; empty `written` omits the `wrote:` block; no-frame session omits the summary.
8. **`TestNoticeClasses`** (F14): `warn` arms an expiry (`noticeExpireMsg` clears it); `setNotice` does not — asserts the documented contract via the seq/expiry mechanics already in place.
9. **Regression**: `go test -race -count=1 ./...` green; `go vet` + `gofmt -l pkg cmd` clean; decoder goldens byte-identical (no `-update`); `blm` command still 469 over `drive_4800.raw`; forbidden-seam diff (`pkg/stream/session.go`, `pkg/stream/stream.go`, `pkg/ecm`, `pkg/decoder`, `pkg/blm`, `go.mod`, `go.sum`) **empty** — all Phase C changes are in `cmd/goaldl` (+ reuse of the unchanged `RecordSink`).

## Suggested implementation slices
1. **Slice 1 — outputs (C.0):** ring buffer + picker + Save Buffer/Log + `saveGrids` subset + `frameCSV.WriteRow` + `% full` chrome; retire `d`. (The user's focus; the rest keys off it.)
2. **Slice 2 — safety (C.1–C.4):** dirty tracking, quit guard, clear undo, exit summary, notice-rule doc.

Each slice is independently shippable and testable; verify-feature can gate them together or in turn.
