<!-- SDA: v1.0 -->
# Evaluation Brief: WinALDL Parity — Phase 3 (Session UX)

You are verifying **Phase 3** of the WinALDL-parity feature for `goaldl` (a Go ALDL scanner/datalogger for the GM 1227747 ECM). Scope: **uncommitted working-tree changes vs `HEAD`** implementing `specs/2026-07-04_feature_winaldl-parity/spec-phase3.md`. Phases 1 (Diagnose) and 2 (Tune) are already merged in `HEAD` and are not under evaluation — only regressions against them are. Evaluate against what was agreed, not against WinALDL feature-completeness.

---

## Section A: What Was Requested

Phase 3 of the WinALDL-parity plan (`plan.md` steps 3.1–3.4) plus one deferred user request:

- **3.1 Recording toggle (`r`)** — the TUI, on a live source, can start/stop writing the raw byte capture to a file mid-session (previously only pre-declared via `record`/`monitor -o`).
- **3.2 Replay pause/speed keys** — `space` pauses/resumes a replay; `+`/`=`/`-` change playback speed at runtime.
- **3.3 Spark-counts grid** — a new grid tab counting knock events (deltas of the cumulative KNOCK_CNT byte, frame offset 17) binned by RPM×MAP.
- **3.4 CSV logging toggle (`d`)** — the TUI can start/stop a decoded-frame CSV log mid-session (previously only `monitor -csv`).
- **Deferred user request** — file-producing actions must prompt for an editable filename (default `goaldl_<ts>`) instead of auto-naming silently.

User decisions (2026-07-04, binding):
1. Filename prompt on **all three** actions: `s` (save grids), `r` (record start), `d` (CSV start). Enter accepts, Esc cancels.
2. Spark grid uses **WinALDL's spark axes**: RPM 400–3600 step 400 × MAP 30–100 step 5 (not the trim-grid axes).
3. **Spark is tab 5**, grouped with the grids: `1 Sensors · 2 BLM · 3 INT · 4 O2 · 5 Spark · 6 Flags · 7 Codes · 8 Raw`.

Post-implementation user feedback (also in scope): no-op key warnings (e.g. `r` during replay) must **self-expire after ~3 seconds** instead of persisting in the footer; newer notices must never be wiped by a stale expiry timer.

Full requirements context: `specs/2026-07-04_feature_winaldl-parity/requirements.md` (deltas D8, D10; success criteria 1–4). The full spec is `specs/2026-07-04_feature_winaldl-parity/spec-phase3.md` — **read it**; it is the agreed contract.

## Section B: Acceptance Criteria

1. **Recording toggle (live)**: with a live source, `r` opens a filename prompt (default `goaldl_<ts>`, `.raw` appended); confirming attaches the capture and the footer shows a red `● REC <name> <bytes>` segment; a second `r` stops, closes the file, and shows a byte-count notice. On a **replay** source `r` is a no-op with an explanatory notice and no prompt.
2. **Recording is fail-soft**: a write error on the capture target (disk full etc.) detaches the recording and surfaces a notice — the live session and its frame stream must survive. The provider-facing `Write` never returns an error.
3. **CSV toggle**: `d` prompts (`.csv`), then writes the same format as `decode`/`monitor -csv` (`time_sec,byte_offset,prom_ok` + one column per parameter) with **one row per ParseOK frame only** (bad frames skipped — monitor parity); stopping shows a row count. Works on live and replay.
4. **Replay pause/speed**: `space` toggles pause (footer shows `⏸ PAUSED`; no frames flow while paused; resume continues from the same data position with no catch-up rush). `+`/`=`/`-` double/halve speed clamped to 0.25×–16× (footer shows `N×` when ≠1×). A speed change applies **from the current position only** (never retroactive). On a live source, or a replay launched with `-speed 0`, these keys are no-ops with explanatory notices. `FrameEvent.Elapsed` stays on the data timeline regardless.
5. **Spark grid**: tab 5 of 8 (keys 1–8, tab/arrows cycle all 8); axes RPM 400–3600/400 × MAP 30–100/5; bins the **mod-256 delta** of the parsed `knock_count` per ParseOK frame (first frame is baseline only; wrap 250→4 reads +10; delta 0 adds nothing); cells display the grid's **Sum** (total knocks), never dimmed; `c` on the spark tab clears the grid but **keeps the knock baseline** (no phantom delta on the next frame).
6. **Filename prompt**: opened by `s`/`r`(start)/`d`(start), pre-filled `goaldl_<ts>`; while open, printable keys — including digits and `q` — type into the buffer (no tab switch, no quit; only ctrl+c quits), backspace edits, Esc cancels with no file written, Enter with an empty buffer cancels. Files are created exclusively (`O_CREATE|O_EXCL` semantics): a name collision keeps the prompt open with a hint and never overwrites.
7. **Save writes four files**: `<base>_BLM.txt` / `_INT.txt` / `_O2.txt` (Phase 2 formats unchanged: BLM/INT have Samples + Wide Average + Correction; O2 has 3-decimal averages and **no** correction) plus `<base>_SPARK.txt` (Samples (frames with knock) + Knock counts, **no** correction).
8. **Loop line gains SPARK**: the persistent loop-status line shows a fifth recording dot `SPARK` with the same ungated condition as O2 (● whenever a good frame exists).
9. **Notice expiry**: the no-op warnings from criteria 1 and 4 self-clear after ~3 s; a stale expiry timer must not clear a newer notice (e.g. a save confirmation issued after the warning).
10. **No regression / architecture invariants**: `pkg/stream/session.go` (`Snapshot`/`Session`), `pkg/ecm`, and `pkg/decoder` are **untouched**; decoder goldens byte-identical (`TestGolden` passes without `-update`); the `blm` command still records **469** closed-loop samples over the drive fixture; `monitor` and `monitor -blm` render unchanged (`BLMBody`/`SensorTable` signatures preserved); no new module dependencies (`go.mod` unchanged).

## Section C: What Changed

Working-tree changes vs `HEAD` (uncommitted; ~1124 insertions / 92 deletions):

| File | Status | Summary |
|---|---|---|
| `pkg/blm/blm.go` | M | `Grid.Sum()` accessor; `SparkRPM`/`SparkMAP`/`NewSpark()` |
| `pkg/blm/blm_test.go` | M | tests for both |
| `pkg/stream/record.go` | **new** | `RecordSink` — mutex-guarded switchable raw-capture tee (fail-soft) |
| `pkg/stream/record_test.go` | **new** | discard/count/swap/error-detach/concurrent(-race) tests |
| `pkg/stream/replay.go` | M | runtime `SetPaused`/`Paused`/`SetSpeed`/`CurrentSpeed`; re-anchored pacing, ≤100 ms wait slices; `Speed==0` inert |
| `pkg/stream/stream_test.go` | M | pause/resume, non-retroactive speed change, unpaced-inert tests (injectable clock) |
| `pkg/stream/gridviews.go` | M | `gridHeat` takes a values matrix; `SparkBody`; `LoopStatus` SPARK dot |
| `pkg/stream/blmview.go` | M | `BLMBody` passes `Average()` to `gridHeat` (behavior identical) |
| `pkg/stream/gridviews_test.go` | M | `TestSparkBody`; LoopStatus cases extended |
| `cmd/goaldl/tui.go` | M | 8 tabs; spark accumulation; filename prompt; `saveGrids(dir, base, …4 grids)`; `r`/`d` toggles; replay keys; footer chrome; notice expiry (`setNotice`/`warn`/`noticeExpireMsg`) |
| `cmd/goaldl/tui_test.go` | M | prompt/spark/record/CSV/replay-keys/notice-expiry tests; 8-tab updates; end-to-end extended |

Docs (not under code evaluation): `specs/…/spec-phase3.md`, `tasks.md`, `README.md`, `product-knowledge/PROJECT_STATUS.md`.

## Section D: How to Verify

From the repo root (`/Users/aaronstone/Development/aldl/goaldl`):

```
go build ./...
go vet ./...
gofmt -l pkg cmd            # must print nothing
go test -race -count=1 ./...
go test ./pkg/decoder -run TestGolden -count=1        # goldens byte-identical, NO -update
go run ./cmd/goaldl blm pkg/decoder/testdata/drive_4800.raw          # expect "Recorded 469"
go run ./cmd/goaldl monitor pkg/decoder/testdata/drive_4800.raw -blm -speed 0   # grid renders, exit 0
go test ./cmd/goaldl -run 'TestTUI|TestSaveGrids' -v                 # the TUI model behavior suite
git status --short && git diff --stat HEAD                          # confirm which files changed
git diff HEAD -- pkg/stream/session.go pkg/ecm pkg/decoder go.mod   # must be EMPTY (forbidden seam)
```

The TUI itself is interactive (no headless driver); behavior is verified through the Bubble Tea model tests in `cmd/goaldl/tui_test.go` — read them critically rather than assuming coverage. The committed real capture `pkg/decoder/testdata/drive_4800.raw` is the ground-truth fixture.

## Section E: Standards to Enforce

Read each from `product-knowledge/standards/`:
- `decoder/byte-value-decoding.md` (must) — decode path must be untouched.
- `decoder/raw-data-policy.md` (must) — no plausibility filtering in decode/emit; gating only at consumers.
- `architecture/session-api-layering.md` (must) — front-ends consume Session/Snapshot; frame-layout knowledge stays in `pkg/ecm`; `pkg/blm` stays generic.
- `testing/golden-fixtures.md` (should) — tests rooted in real captures; goldens regenerated only deliberately.
- `go/tooling.md` (should) — gofmt/vet/build/test -race gate; minimal dependencies.

Core philosophies (blocking if violated), from `product-knowledge/philosophies/`: `consolidate-over-accrete.md`, `ground-truth-first.md`.

## Section F: Personas to Consult

Read each from `product-knowledge/personas/` and review from that perspective:
- `product-manager.md` — scope matches the agreed Phase 3 cut + the three user decisions; no creep; user value at the car.
- `architect.md` — layering (controls on providers below the Session facade), no new deps, `blm` generality, consolidation vs forking.
- `qa.md` — edge cases (knock wrap/reset, empty grids, prompt collisions, sink write errors, pause during CSV, quit with open files), test coverage vs the spec's §8 test plan, regression safety.
