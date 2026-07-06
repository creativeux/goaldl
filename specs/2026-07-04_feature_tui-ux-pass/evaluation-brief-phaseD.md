<!-- SDA: v1.0 -->
# Evaluation Brief: TUI UX Pass — Phase D (Replay & startup ergonomics)

You are evaluating **Phase D** of the TUI UX Pass epic. Phases A–C already shipped; Phase D is the replay/startup slice. Everything below is self-contained — do not assume knowledge of how it was built.

Repo: `goaldl` (Go). Branch under test: `feat/tui-phase-d` (cut from `main`). The implementation is **uncommitted in the working tree** — evaluate the working tree, diff it against `main`.

---

## Section A — What Was Requested

Phase D remediates three findings from the TUI heuristic evaluation (requirements in `specs/2026-07-04_feature_tui-ux-pass/requirements.md`):

- **F6 — Replay has controls but no position.** Pause/speed exist; there is no position/duration/percent display and no seek. Analyzing a 14-min capture means watching it in full.
- **F7 — Multi-port startup dead-ends.** Auto-connect requires exactly one port; with 2+ ports the generic "No source" error doesn't even say ports were found.
- **F8 — "waiting for frames…" is a diagnostic dead zone.** Live, before first sync, nothing indicates whether ANY bytes are arriving — the is-it-the-cable-or-the-baud question is unanswerable from the screen.

The three relevant **success criteria** (from requirements.md):
6. Replay footer shows position / total / percent; seek (±10 s) and restart work; live source unaffected.
7. `goaldl` with 0 or 2+ ports lists the detected ports and how to choose (or offers an in-TUI picker).
8. The waiting screen distinguishes "no bytes at all" from "bytes but no frame sync" with rate shown.
11. **Regression**: decode path untouched (goldens byte-identical); `Snapshot`/`Session` API unchanged except where a phase explicitly names a provider-level addition; `blm` command still records 469 over the drive fixture; full suite green under `-race`.

**Non-goals** (must NOT appear): grid rewind on backward seek; scrub-to-arbitrary-position/mouse; a ±60 s seek tier; persisting the chosen port/config file; live re-connection mid-session; any `Snapshot`/`Session`/`ecm`/`decoder`/`blm`/`go.mod`/`go.sum` change (only provider methods are allowed below the facade).

---

## Section B — What Was Agreed To (acceptance criteria)

Full spec: `specs/2026-07-04_feature_tui-ux-pass/spec-phaseD.md`. Acceptance criteria distilled:

**D.1 Replay position + seek**
1. `ReplayProvider.Duration()` returns the capture's total data-timeline length (last frame's Elapsed), 0 for empty; O(1); callable before `Run`; never disagrees with a full `Run`'s last Elapsed.
2. `ReplayProvider.Seek(target)` clamps to `[0, Duration]`, is a **no-op when `Speed==0`** (unpaced), and applies at the next frame boundary — a **forward** seek jumps ahead and continues; a **backward** seek re-emits earlier frames; a seek **while paused** shows the target frame then holds.
3. TUI keys (replay only, routed through the existing `replayGuard`): `,` = −10 s, `.` = +10 s, `0` = restart; each clamped to the capture length. On a **live** source or an **unpaced** replay the keys warn and never seek.
4. The replay playback row shows the position; the position/total/percent readout (`m:ss / m:ss (N%)`, **no `t=` prefix**) leads the **header status** (left of `Signal:`). *(Note: the header placement + `t=` removal + speed-ladder were user follow-ons during implementation — see Section C.)*
5. Backward seek does **NOT** rewind the consumer grids/extrema (they are whole-session aggregates; `c` clears). The frame ring likewise keeps accumulating.
6. `+`/`-` step a **fixed speed ladder** (`0.25·0.5·1·2·4·8·16×`), not a ×2/÷2 factor, so an off-ladder start (e.g. `-speed 5`) can reach 1× — stepping to the nearest stop in that direction, clamped at the ends.

**D.3 Waiting-screen byte diagnostics**
7. `SerialProvider.Bytes()` returns total raw bytes read (atomic; safe concurrent read); increments after each non-zero `Read`.
8. The live pre-first-frame waiting screen shows: `bytesSeen==0` → a cable/port/driver hint; `bytesSeen>0` (still no frame) → a baud/polarity hint **with a B/s rate**. A **replay** source shows the bare wait unchanged. Once a frame arrives, neither diagnostic renders.

**D.2 Port discovery UX**
9. `launchTUI`: bare `goaldl` with **exactly one** port auto-connects (unchanged); with **0 or 2+** ports on an interactive terminal it opens an **in-TUI port picker**; a chosen port connects; declining exits 0. A non-interactive stdin (piped) falls through to the improved `errNoTUISource` (which now **lists detected ports**). An explicit `-p`/capture-file arg is never intercepted.
10. The `portPicker`: re-polls `AvailablePorts` on a 1 s tick, **auto-connects when the count drops to exactly 1**, `↑/↓` move a clamped cursor, `enter` returns the highlighted port, `q`/`ctrl+c` decline (return ""); 0 ports shows a retry + PL2303-driver hint; a scan error is surfaced and polling continues. Uses a **stdlib** TTY check (`os.Stdin.Stat()` char-device), **no new dependency**.

---

## Section C — What Changed

Diff against `main` (working tree). Read the files directly.

Tracked:
```
 cmd/goaldl/main.go        |  13 ++-
 cmd/goaldl/tui.go         | 196 ++++++++++++++++++++++++++++++++--
 cmd/goaldl/tui_test.go    | 171 +++++++++++++++++++++++++++++
 pkg/stream/replay.go      | 124 ++++++++++++++++++++++--
 pkg/stream/serial.go      |   9 +++
 pkg/stream/stream_test.go | 197 +++++++++++++++++++++++++++++++++++
```
New (untracked) files:
```
 cmd/goaldl/portpicker.go       (portPicker model + runPortPicker + stdinIsInteractive)
 cmd/goaldl/portpicker_test.go  (TestPortPicker)
```

Summary of changes:
- **`pkg/stream/replay.go`** (below-facade provider): cached one-time decode shared by `Run`+`Duration()`; `Seek`/`PendingSeek`/`takeSeek`/`seekIndex`/`frameElapsed`; `Run` rewritten to an index loop applying a pending seek at the frame boundary with re-anchor.
- **`pkg/stream/serial.go`** (below-facade): `nbytes atomic.Int64` + `Bytes()`; one `Add` after a non-zero `Read`.
- **`cmd/goaldl/tui.go`**: `replayTotal`, `serial byteSource` (interface for testability), `seekBy`/seek keys, `replayPosition` (in the header, no `t=`), slimmed `replayNav`, fixed-ladder `adjustSpeed`, `bytesSeen`/`byteRate` tick sampling, `waitingBody` diagnostics, `errNoTUISource` lists ports.
- **`cmd/goaldl/main.go`**: `launchTUI` opens the picker on ambiguous ports.
- **`cmd/goaldl/portpicker.go`**: the pre-session picker model.
- Tests added in `stream_test.go`, `tui_test.go`, `portpicker_test.go`.

**Watch specifically for** (things worth an independent check, not just trusting the diff):
- Any change to `Snapshot`/`Session`/`pkg/ecm`/`pkg/decoder`/`pkg/blm`/`go.mod`/`go.sum` (there must be **none** — this is a hard constraint).
- The typed-nil-in-interface trap on `m.serial` (a nil `*SerialProvider` must not become a non-nil `byteSource` on a replay source — confirm the guarded assignment in `cmdTUI`).
- Seek re-anchoring correctness: does a forward seek actually skip intermediate frames, and does playback pace correctly afterward (not rush/stall)?
- Does the port picker hang a non-TTY invocation? (It must fall through to stderr.)

---

## Section D — How to Verify

Test/lint (the project's whole gate — `go/tooling.md`):
```
gofmt -l pkg cmd          # expect no output
go vet ./...
go build ./...
go test -race -count=1 ./...
```
Feature-specific tests: `TestReplaySeek`, `TestReplayDuration` (pkg/stream); `TestTUISeekKeys`, `TestWaitingDiagnostics`, `TestSerialBytes`, `TestTUIReplayKeys` (cmd/goaldl); `TestPortPicker` (cmd/goaldl).

Regression / seam guards (all must hold):
```
go test ./pkg/decoder -run TestGolden -count=1        # goldens byte-identical, NO -update
go run ./cmd/goaldl blm pkg/decoder/testdata/drive_4800.raw   # must report "Recorded 469"
git diff --stat main -- pkg/stream/session.go pkg/stream/stream.go pkg/ecm pkg/decoder pkg/blm go.mod go.sum   # MUST be empty
git diff --name-only main -- pkg/stream/                # only replay.go + serial.go (+ stream_test.go)
```
Confirm `Snapshot` gained no field: `git diff main -- pkg/stream/ | grep -i 'Snapshot struct' -A20` (should show no field additions).

App interaction (no browser; it's a terminal TUI): you may render the model headlessly by constructing a `tuiModel` in a throwaway `_test.go` and calling `.View()` / `.replayNav()` / `.waitingBody()` / driving `.Update(msg)` — see the existing `tui_test.go` for the idiom (`testModel()`, `runes()`), or drive a real replay via `go run ./cmd/goaldl pkg/decoder/testdata/drive_4800.raw` (interactive; `,`/`.`/`0`/`space`/`+`/`-`, `q` to quit). Judge whether the observed behavior matches the criteria, not just that tests pass.

---

## Section E — Standards to Enforce

Read each and check the diff against it:
- `product-knowledge/standards/architecture/session-api-layering.md` — front-ends consume `Session`/`Snapshot`; provider additions are allowed *below* the facade, but `Session`/`Snapshot` must stay unchanged. **Most relevant** — Phase D adds provider methods.
- `product-knowledge/standards/decoder/raw-data-policy.md` — no plausibility filtering in the decode path; quality signals ride alongside as fields. (D.3's byte counter is a diagnostic field, not a filter; seek re-emits frames unfiltered.)
- `product-knowledge/standards/go/tooling.md` — gofmt+vet+build+`test -race` is the whole gate; minimal dependencies (the TTY check must be stdlib — verify no new `go.mod` entry).
- `product-knowledge/standards/testing/golden-fixtures.md` — decoder correctness proven against the committed real captures; goldens byte-identical unless an intentional decoder change (there is none here).

## Section F — Personas to Consult

Adopt each and critique:
- `/Users/aaronstone/.claude/plugins/cache/crux-marketplace/glados/1.3.0/src/personas/architect.md` (layering, standards, performance)
- `/Users/aaronstone/.claude/plugins/cache/crux-marketplace/glados/1.3.0/src/personas/product-manager.md` (user value, scope, non-goals)
- `/Users/aaronstone/.claude/plugins/cache/crux-marketplace/glados/1.3.0/src/personas/qa.md` (edge cases, failure modes, coverage)

Write your evaluation to `specs/2026-07-04_feature_tui-ux-pass/evaluation-phaseD.md`.
