<!-- SDA: v1.0 -->
# Requirements: TUI UX Pass — Heuristic Analysis Remediation

**Goal**: Make the dashboard the most useful and efficient at-the-car instrument it can be, by fixing the trust, layout, and safety defects surfaced by a full heuristic evaluation of the TUI (2026-07-04).

**Who**: The owner-tuner operating at the vehicle — often on a laptop with a small terminal, engine running, divided attention. Their session data (grids accumulated over a drive) is the product; their first-run experience (does the cable work?) is the adoption gate.

**Method**: Nielsen-heuristic walkthrough of `cmd/goaldl/tui.go` + `pkg/stream` view builders, grounded by rendering every tab from the committed drive fixture (`pkg/decoder/testdata/drive_4800.raw`, 635 frames) and cross-checking suspicious data against the WinALDL ground-truth log (`data/20250601_111156_LOG.txt`).

---

## Findings

Severity: **S1** actively misleads or hides failure · **S2** loses data or blocks the task · **S3** friction/inefficiency · **S4** polish.

### S1 — Trust & failure visibility

| # | Finding | Heuristic | Evidence |
|---|---------|-----------|----------|
| F1 | **Live-source failure is silent.** `cmdTUI` discards `session.Run`'s error (goroutine at `tui.go:147-155`); a failed port open (wrong name, missing PL2303 driver, unplugged cable — the most likely first-run failures) renders as `waiting for frames… (stream ended)` with no error text. | Help users recognize & diagnose errors | Code path: `session.Run` return value unread; only `close(snaps)` → `providerDoneMsg` reaches the model |
| F2 | **Spark tab presents counter artifacts as knock data on this vehicle.** `KNOCK_CNT` free-runs at ~+76/frame while driving — verified in BOTH the drive capture and the WinALDL ground-truth log — so the grid shows cells like 6232 "knocks" directly under an explainer saying "the goal is 0 everywhere". An operator trusting it would pull timing everywhere. Needs consumer-side free-running detection + status warning (raw data stays displayed — raw-data-raw policy assigns exactly this call to the view layer). | Match between system & real world | Fixture render: spark cells 76–6232; ground-truth log RAW:KNOCK_CNT column increments ~76/frame mod 256 |
| F3 | **No data-staleness indication.** If frames stop live (ignition off, cable bump), the last frame stays rendered indefinitely, heartbeat stays green, `t=` freezes without comment. Frame cadence is a known ~1.2 s, so "stale" is well-defined. | Visibility of system status | `latest` only updates on `snapshotMsg`; no timer in the model |

### S2 — Data loss & task blockage

| # | Finding | Heuristic | Evidence |
|---|---------|-----------|----------|
| F4 | **Layout doesn't adapt to terminal size; the footer dies first.** `View()` concatenates header+body+footer; alt-screen truncates the bottom. BLM/INT tabs are ~29 lines (16 grid rows + 6-line explainer + chrome), Raw ~28 — on 80×24 the footer (key legend, ● REC indicator, notices, **the filename prompt itself**) is invisible. Width: spark grid 83 cols, 6-col sensor table ~100 cols — both wrap and garble at 80. `m.width/m.height` tracked but only Raw uses width. | Visibility of system status / flexibility | Line counts from fixture renders; `tui.go View()` uses no height clamp |
| F5 | **Destructive actions have no guard.** `q` quits instantly with recording running and grids unsaved; `c` irreversibly clears the active grid. After a 45-minute drive the BLM grid IS the session's product — one keystroke destroys it. | Error prevention | `tui.go:313-315` (quit), `clear()` at `tui.go:464` |
| F6 | **Replay has controls but no position.** Pause/speed exist; there is no position/duration/percent display and no seek. Total duration is computable (`len(data)/160` seconds). Analyzing a 14-min capture means watching it. | Visibility / flexibility & efficiency | `sessionChrome()` shows only ⏸/N×; `ReplayProvider` has no seek |
| F7 | **Multi-port startup dead-ends.** Auto-connect requires exactly one port; with 2+ ports the generic "No source" error doesn't even say ports were found, or which. | Recognition rather than recall | `launchTUI` (`main.go:56-63`), `errNoTUISource` message |
| F8 | **"waiting for frames…" is a diagnostic dead zone.** Live, before first sync, nothing indicates whether ANY bytes are arriving — the classic is-it-the-cable-or-the-baud question is unanswerable from the screen. | Visibility of system status | No byte-level signal crosses the Provider→Session seam pre-frame |

### S3 — Friction & confusion

| # | Finding | Heuristic |
|---|---------|-----------|
| F9 | **"rec:" collision**: loop line's `rec: BLM ● INT ● …` (grid accumulation) vs footer's `● REC` (raw file recording) — two unrelated meanings of "recording" in the same chrome. | Consistency & standards |
| F10 | **Context-blind key legend**: footer always shows all keys, but `c` is modal per tab (clear grid / reset min-max / no-op), `space/±` replay-only, `r` live-only. No `?` help overlay for the rest. | Recognition rather than recall |
| F11 | **Codes/Flags are instantaneous-only**: a code that sets transiently mid-drive and clears is missable. A "seen this session" latch (consumer-side) matches how a tuner uses codes. | Visibility of system status |
| F12 | **MIN/MAX polluted by known-garbage frames**: the drive fixture's 0 V-battery tail frames put MIN 0 V / battery, 221 °F coolant into extrema permanently. ParseOK gating isn't enough; PROM-gating extrema is a legitimate consumer-level quality decision under the raw-data policy. | Match with real world |
| F13 | **Vertical waste in grids**: RPM rows 4000–6400 essentially never populate on this engine; 7 dead rows are a major cause of F4's footer clipping. Explainers (deliberate, user-requested — keep default-on) cost 5–6 lines with no way to reclaim them on small terminals. | Aesthetic & minimalist design |
| F14 | **Notice lifecycle inconsistency**: warnings self-expire (3 s), action notices persist until replaced — unpredictable. | Consistency & standards |
| F15 | **Stale help text**: `goaldl help` says "keys: 1-3 / tab switch views" (`main.go:72`) — two phases out of date; s/c/r/d/space unmentioned. | Help & documentation |

### S4 — Polish

| # | Finding |
|---|---------|
| F16 | Heartbeat is color-only (green/red ●) — pair with a glyph change for colorblind operators (PROM ✓/✗ already does this right). |
| F17 | Filename prompt: no cursor movement (append/backspace only) and no destination-directory display (at the car you may not know your cwd). |
| F18 | `s` saves all four grids only; no single-grid save. |
| F19 | Current-reading status lines (e.g. O2 tab) show tail-garbage values without qualification — same family as F3/F12. |

## Success criteria

1. A failed port open shows the actual error in the TUI (and on stderr on exit) — never a bare "waiting for frames…". *(F1)*
2. Pulling the cable mid-session produces a visible staleness signal within ~5 s (≈3 missed frames); heartbeat stops reading "good". *(F3)*
3. On the ground-truth vehicle data, the Spark tab states that the knock counter is free-running and not a usable knock signal; grid values remain visible (raw-data-raw). On a crafted capture with genuine sparse knock deltas, no warning appears. *(F2)*
4. On an 80×24 terminal, every tab shows header, loop line, and full footer; clipped body content is indicated, never silently lost; no line wraps. *(F4)*
5. `q` with an active recording/CSV or unsaved grid data requires confirmation; `c` is confirmable or undoable; neither costs more than one extra keystroke in the safe case. *(F5)*
6. Replay footer shows position / total / percent; seek (±10 s) and restart work; live source unaffected. *(F6)*
7. `goaldl` with 0 or 2+ ports lists the detected ports and how to choose (or offers an in-TUI picker). *(F7)*
8. The waiting screen distinguishes "no bytes at all" from "bytes but no frame sync" with rate shown. *(F8)*
9. Legend/help: `?` overlay exists; footer legend is context-correct per tab and source. *(F10)*
10. `goaldl help` matches the shipped dashboard. *(F15)*
11. Regression: decode path untouched (goldens byte-identical); `Snapshot`/`Session` API unchanged except where a phase explicitly names a provider-level addition; `blm` command still records 469 over the drive fixture; full suite green under `-race`.

## Non-goals

- The `serve` adapter, Dash big-number view, config persistence, multi-ECM (existing Phase-4/deferred items — separate features).
- Mouse support.
- Any change to saved grid/CSV/raw file formats.
- Any plausibility filtering in the decode path (raw-data-raw): F2/F12 remediations are strictly consumer/view-level annotation or gating.
- Removing the grid explainers (user-requested feature; F13 only adds a reclaim mechanism).
