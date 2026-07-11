<!--
SDA: v1.0
GLaDOS-MANAGED DOCUMENT
Last Updated: 2026-07-11
To modify: Edit directly.
-->

# Roadmap

Mission: a working, cross-platform scanner/datalogger/tuning aid for GM 160-baud ALDL ECMs (primary target: GM 1227747), in Go, over an ordinary USB serial adapter. See [MISSION.md](MISSION.md).

**Positioning (2026-07-11)**: goaldl is the **car-side complement to TunerPro RT**, not its replacement. goaldl captures and preps; TunerPro edits the bin. The sweet spot is **onboard logging + data prep for tuning** — everything below WinALDL parity flows toward that. Competitive analysis trace: `specs/2026-07-11_plan-product/`.

## Phase 0: Decode & consolidate — ✅ done (2026-07-03)

- [x] Byte-value decoder (one UART byte per ALDL bit), unit- + hardware-validated (635/635 PROM match on a real drive)
- [x] record / decode / monitor / blm / simulate pipeline
- [x] Consolidation to the working core; test suite rooted in real captures + golden frame dumps
- [x] Core `stream.Session` → `Snapshot` API; TUI dashboard as the default face

## Phase 1: WinALDL parity — diagnose — ✅ shipped (2026-07-04)

**Goal**: at the car, read everything the ECM says without WinALDL.

- [x] Flags / error-codes / knock decoded as `pkg/ecm` data tables (A033.ads bit order, log-verified); `Snapshot` carries Flags/Codes
- [x] TUI: Flags, Codes, scrolling raw-history tabs; heartbeat + bad-sample gating
- [x] Sensor table dual-unit (TPS % via `-tps0`/`-tps100`, MAP kPa); `MapVoltsToKPa` corrected to the WinALDL-verified transfer

## Phase 2: WinALDL parity — tune — ✅ shipped + verified (2026-07-04)

**Goal**: a live tuning session needs no post-hoc `blm` run.

- [x] INT grid (closed-loop gated) and O2 grid (ungated) tabs — `blm.Grid` reuse
- [x] In-TUI Save (`s`, all three grids → timestamped files) / Clear (`c`, active grid or sensor Min/Max)
- [x] Always-on sensor MIN/MAX columns
- [x] Persistent loop-state line on every tab (Open/Closed + per-grid recording dots)
- [x] Consumer-side accumulation only — no `Snapshot`/`Session`/`blm`/`ecm` change; decode path untouched

## Phase 3: WinALDL parity — session UX — ✅ shipped + verified (2026-07-04)

- [x] In-TUI recording toggle (`r`) — `stream.RecordSink` behind the existing `SerialProvider.Sink` seam; fail-soft on write errors (detach + notice, session survives)
- [x] Replay pause / speed keys (space, +/-/=) — runtime `ReplayProvider` controls, re-anchored non-retroactive pacing, 0.25×–16×
- [x] Spark-counts grid (knock-delta accumulator, mod-256 wrap) on WinALDL's spark axes (RPM 400–3600/400 × MAP 30–100/5), tab 5 of 8
- [x] In-TUI CSV logging toggle (`d`, reuses `csv.go`; ParseOK rows — `monitor -csv` parity)
- [x] Filename prompt on all file actions (`s`/`r`/`d`, default `goaldl_<ts>`, exclusive-create, no silent overwrite) — fulfils the deferred editable-filename request; no-op key warnings self-expire after 3 s
- [x] `Snapshot`/`Session`/`ecm`/decoder untouched; `pkg/blm` gains only generic `Sum()`/`NewSpark()`; zero new dependencies

## TUI UX pass — 🔨 in progress (2026-07-04)

**Goal**: make the dashboard the most useful, trustworthy at-the-car instrument it can be. Five-phase plan from a full heuristic evaluation (19 findings F1–F19); spec: `specs/2026-07-04_feature_tui-ux-pass/`. Presentation/consumer-only — no `Session`/`Snapshot`/`ecm`/`decoder`/`blm` change.

- [x] **Phase A — Trust** ✅ shipped + verified (2026-07-04): live session errors surface as a full-screen diagnosis panel + stderr on exit (F1, was silently discarded); staleness heartbeat (hollow `○` + `no data Ns` after ~6 s quiet, F3); free-running-knock detection warns on the Spark tab while keeping the raw counts visible (F2 — verified against both the drive fixture and the WinALDL log); help text corrected (F15)
- [x] **Phase B — Layout resilience** ✅ shipped (2026-07-04; B.1+B.4 verified, B.2 awaiting verify): pinned tab bar/loop line/footer with a height-clamped scrollable body (`j`/`k`/↑/↓) so a short terminal never scrolls the tabs off the top (F4); grid explainers collapsed behind an `i` info accordion; **B.2 width awareness** — narrow terminals truncate with a `›` cue (grids at whole-column boundaries, sensor table drops ALT→RAW, ANSI-aware chrome fit) instead of wrapping. B.3 (collapse empty rows) implemented then reverted at user request. Sub-6-row degenerate height deferred.
- [x] **Phase C — Session safety + unified outputs** *(Slice 1 merged PR #6; Slice 2 impl 2026-07-05)*: WinALDL-style output checklist collapsing `s`/`r`/`d` into **Save Buffer** (retroactive, bounded decoded-frame ring, no RAW) + **Log** (`l`, forward, crash-tolerant atomic grid writes, RAW optional); GoALDL title bar + `Signal:` dot + mode-aware chrome; dirty-grid tracking, quit confirm, clear undo (`u`). (Exit summary descoped.) Consumer-side only
- [x] **Phase D — Replay & startup** ✅ shipped + verified (2026-07-05; fresh evaluator PASS 10/10): replay position `m:ss / m:ss (N%)` in the header + `,`/`.` ±10s + `0` restart (provider `ReplayProvider.Seek`/`Duration`, frame-boundary jump; backward seek leaves grids as-is); `+`/`-` fixed speed ladder; waiting-screen byte diagnostics (`SerialProvider.Bytes()` → cable vs baud/polarity with B/s); in-TUI port picker on 0/2+ ports (auto-connect at 1, stdlib TTY guard). Only below-facade provider additions; Session/Snapshot untouched, goldens byte-identical, `blm` 469.
- [ ] **Phase E — Learnability**: `?` help overlay, context-sensitive footer, codes/flags session latch, PROM-gated extrema

## Horizon 1: Data prep for tuning — ⏳ next (after TUI UX pass)

**Goal**: close the drive → paste-into-TunerPro loop. goaldl already computes what tuners do by hand in TunerPro (BLM histogram → mental VE correction); make that output land directly in the bin editor.

- [ ] **XDF-aware correction export**: read the community XDF for the bin being tuned (read-only parse — table axes, addresses, scaling), re-bin the BLM correction grid onto the actual VE/fuel table's axes, emit TunerPro's tab-delimited paste format. `goaldl blm session.raw --xdf 747.xdf` → paste → multiply.
- [ ] **Suggested-change report**: per-cell current × correction = proposed value, with sample counts and confidence-threshold flags, reviewable before pasting.
- [ ] **Session report**: post-drive one-shot summary — knock events with RPM/MAP/time context, open/closed-loop time split, under-threshold cells ("drive more at 2000 RPM / 60 kPa"), warm-up curve, extrema.
- [ ] **Cross-session correction diff**: did the last bin change move cells toward 128? Diff two sessions' correction grids — direct feedback on whether the TunerPro edit worked.

## Horizon 2: Onboard logging — ⏳ bridge first (ESP32-S3 in flight)

**Goal**: the car logs itself; no laptop rides shotgun. Bridge path first (hardware ordered, spec held), Pi daemon later reusing the same headless features.

- [ ] **TCPProvider** — specced + held for hardware (`specs/2026-07-06_feature_tcp-provider/`): ESP32-S3 bridge streams raw bytes over WiFi/TCP; goaldl consumes via `-tcp host:port`. Implementation resumes when the S3 arrives (real-bridge validation is the ground-truth step).
- [ ] `serve` adapter (HTTP/WebSocket) proving the `Session` API drives a non-terminal front-end — inherits flags/codes/grids for free from `Snapshot`
- [ ] Phone-bracket live dashboard (gauges + BLM grid) on the `serve` adapter; Dash (big-number) view
- [ ] **Headless record mode** (later, Pi Zero-class): daemonized `record` — start on boot, file rotation, minimal heartbeat signal; pure Go on the existing serial path
- [ ] Optional onboard MCU datalogging bridge, falling-edge method (superseded in the near term by the ESP32-S3 UART approach; keep as fallback)

## Horizon 3: Interop & breadth — ⏳ demand-driven

**Goal**: speak the community's formats so goaldl slots into existing workflows and covers more ECMs without hand-porting.

- [ ] **ADX import**: derive `pkg/ecm` definitions from TunerPro ADX (XML) files — gets the 1227165 and siblings for free, keeps conversions identical to what the user sees in TunerPro
- [ ] **Log format interop**: export decoded frames in formats TunerPro's playback/histograms accept; optional WinALDL log format (we already match its ground truth)
- [ ] Config-file persistence (port / ECM / TPS calibration)
- [ ] Additional ECM definitions (data-only, demand-driven — via ADX import above where possible)

**Permanent non-goals**: bin/hex editing, XDF table editing, emulation, PROM burning/reading, checksum plugins (TunerPro's half of the partnership — 2026-07-11); Narrow/Avg10/StdDev grid modes; Windows-dialog config UX; any plausibility filtering in the decode path (raw-data-raw).
