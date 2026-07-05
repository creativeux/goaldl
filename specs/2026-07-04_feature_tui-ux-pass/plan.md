<!-- SDA: v1.0 -->
# Plan: TUI UX Pass

Five phases, ordered so each is independently shippable and the highest-severity findings land first. Recommended cut: **A → B → C now**; D and E can ride with (or after) the `serve` adapter work since nothing in them touches `Session`/`Snapshot`.

Architecture stance (continuing the parity-phase pattern): the `Session`/`Snapshot` facade stays untouched; the only core additions sit on **providers below the facade** (error surfacing, byte counter, seek — the same seam that took `RecordSink` and pause/speed in parity Phase 3). Everything else is `cmd/goaldl/tui.go` model/View work and pure builders in `pkg/stream`.

---

## Phase A — Trust (S1 findings; small diffs)

**A.1 Surface session errors (F1).** The `cmdTUI` session goroutine captures `session.Run`'s error and delivers it to the model (new `errorMsg` alongside `providerDoneMsg`). The TUI renders it as a prominent body/footer banner ("serial: open /dev/cu.usbserial-10: no such file — check `goaldl ports`"), and `cmdTUI` re-prints it to stderr after the alt-screen closes so it survives quit. No provider change — the error already propagates to `Run`'s return; it is currently dropped on the floor.

**A.2 Staleness indicator (F3).** A repeating `tea.Tick` (1 s) compares wall time against the last snapshot arrival. Beyond ~3.6 s (3 × frame cadence) on a live source: heartbeat hollows (`○`), footer gains `no data 8s`. Replay pause is exempt (expected staleness). Pure model/View change.

**A.3 Free-running knock-counter detection (F2).** Consumer-side, in the TUI's spark accumulation: track the fraction of parsed frames with a nonzero knock delta over a sliding window (e.g. last 50 parsed frames). Above a threshold (~50 %), `SparkBody`'s status line shows a persistent warning ("KNOCK_CNT advancing every frame — free-running counter, not knock; values are not meaningful on this vehicle") and the explainer gains one line on the failure mode. Grid keeps rendering (raw-data-raw: annotate, never hide). Oracle: ground-truth log (fires) vs crafted sparse-knock capture (doesn't).

**A.4 Fix stale help text (F15, part of F10).** `printUsage` dashboard section: 8 tabs, session keys, `?` mention once E lands.

## Phase B — Layout resilience (F4, F13)

**B.1 Pinned chrome, clamped body.** Restructure `View()`: header + loop line pinned top, footer pinned bottom, body gets `height - chrome` lines. Overflow truncates with a `… +N lines (j/k scroll)` marker; `j/k` (or PgUp/PgDn) scroll the body. Bubble Tea `WindowSizeMsg` already feeds `m.width/height`.

**B.2 Width awareness.** Wide bodies (spark grid, 6-col sensor table) get a right-edge truncation cue instead of wrapping; sensor table may drop the ALT column first, then RAW, under pressure (order chosen in spec).

**B.3 Collapse trailing empty grid rows (screen only).** `gridHeat` gains an option to elide trailing all-empty RPM rows behind one `(rows 4000–6400 empty)` line. Saved files unchanged.

**B.4 Explainer toggle.** `e` collapses the grid explainer to its one-line legend and back; default expanded (explainers were user-requested). Persisting the choice is out of scope (no config file yet).

## Phase C — Session safety (F5, F14)

**C.1 Dirty tracking.** Model tracks unsaved-grid state (any grid add since last `s`/start) and open outputs.

**C.2 Quit guard.** `q` with recording/CSV open or dirty grids → footer confirm ("unsaved grids, recording active — q again to quit, s to save"); second `q` (or timeout) quits. `ctrl+c` stays immediate (escape hatch).

**C.3 Clear guard.** Either confirm-style (`c` then `c`) or one-slot undo (`c` clears, notice "cleared BLM (u to undo)", undo restores the grid pointer). Decision for spec-feature; undo is one retained pointer, likely the cheaper UX.

**C.4 Exit summary + notice rule.** After alt-screen close, print a session summary (frames ok/bad, files written with sizes/rows). Notice lifecycle rule made deliberate: warnings expire, action notices persist — documented in code, or unified if the spec review prefers.

## Phase D — Replay & startup ergonomics (F6, F7, F8)

**D.1 Replay position + seek.** Footer: `t=34s / 812s (4%)`. `ReplayProvider` precomputes total duration and gains `Seek(±d)` — frames are already decoded up front, so seek is an index jump + pacing re-anchor (same mutex/anchor mechanics as pause/speed; provider-level, below the facade). Keys: `,`/`.` ±10 s, `0` restart. Consumer-side grid state is NOT rewound on backward seek (grids accumulate a session, not a timeline — documented; `c` exists).

**D.2 Port discovery UX.** No-source error lists detected ports. Stretch (spec decision): with 0/2+ ports, enter the TUI in a picker/waiting state instead of exiting — retry `AvailablePorts` on a tick, arrow-select, Enter connects.

**D.3 Waiting-screen byte diagnostics.** `SerialProvider` counts raw bytes read (atomic, `Bytes()` — mirrors `RecordSink.Bytes()`); TUI polls it on the staleness tick while `!hasFrame`: `0 bytes — check cable/port` vs `159 B/s, no frame sync — check baud (-b) / polarity (-invert)`.

## Phase E — Learnability (F9, F10, F11, F12, F16–F19)

**E.1 `?` help overlay** — full key map + tab glossary, any key closes.
**E.2 Context-sensitive footer legend** — per-tab `c` label, replay/live keys only when applicable.
**E.3 Terminology** — loop line `rec:` → `learn:` (or `grids:`); heartbeat glyph differs when bad, not just color (F16).
**E.4 Codes/flags session latch** — consumer-side "seen this session" (`[!] 44 O2 lean — seen t=312s, now clear`), cleared with `c` on those tabs.
**E.5 PROM-gated extrema (F12)** — MIN/MAX (and current-reading status lines, F19) accumulate only from PROM-OK frames; footnoted in the table. Consumer-level, policy-compliant. Decision for spec-feature.
**E.6 Prompt polish (F17, F18)** — show destination directory in the prompt line; optional cursor movement; optional per-grid save modifier.

## Sequencing & risk

- A is independent and tiny; ship first (it changes trust at the car immediately).
- B is the largest chunk and purely presentational; everything after it renders inside its frame, so it precedes C–E visually but C does not depend on it.
- Provider-level additions (A.1 error already exists; D.1 seek; D.3 byte counter) are the only `pkg/stream` non-builder changes — same seam as parity Phase 3, no `Session`/`Snapshot` change anywhere.
- Test strategy per precedent: model-level tests driving real `ReplayProvider`+`Session` over the drive fixture; crafted captures for staleness/knock/seek oracles; goldens byte-identical throughout.

## Open decisions for spec-feature

1. `c` guard style: confirm vs one-slot undo (C.3).
2. PROM-gated extrema: yes/no, and whether current-reading status lines gate too (E.5).
3. Port picker in-TUI vs error-text-only (D.2).
4. Body overflow: scroll keys (`j/k`) vs truncation-marker-only (B.1).
5. Sensor-table column-drop order under width pressure (B.2).
6. Spark free-run warning threshold and whether it also suppresses the explainer's "goal is 0" line (A.3).
