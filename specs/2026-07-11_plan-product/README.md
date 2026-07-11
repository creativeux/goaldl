# Plan Product — 2026-07-11

**Trigger**: User direction following a TunerPro RT competitive analysis: reposition goaldl as the
*complement* to TunerPro RT rather than a replacement — "onboard logging and data prep for tuning
is the sweet spot."

**Scope**: Update `product-knowledge/ROADMAP.md` with the complement-positioning horizons
(onboard/headless logging, XDF-aware correction export, ADX definition import, log interop,
session reports); declare bin editing / emulation / PROM burning permanent non-goals.
Create `product-knowledge/TECH_STACK.md` (none existed; stack currently summarized only in
PROJECT_STATUS.md).

## Session log

- Prerequisites verified: MISSION.md (confirmed 2026-07-04), PROJECT_STATUS.md current.
- Competitive analysis input (this session, web research): TunerPro RT = scanner (ADX) +
  bin editor (XDF) + emulation front-end (Moates hardware); Windows-only; 160-baud logging
  is its documented weak spot. goaldl already ahead on 160-baud fidelity, raw-first capture,
  BLM correction computation, cross-platform. Complement thesis: goaldl = car-side instrument,
  TunerPro RT = desk-side editor; interop (XDF/ADX/log formats) is the bridge.
- User decisions (2026-07-11):
  1. **First horizon = data prep for tuning** (XDF-aware correction export before onboard logging
     or interop foundation) — highest leverage, pure software, no hardware dependency.
  2. **Onboard platform = both, bridge first** — ESP32-S3 + TCPProvider (already in flight,
     spec held for hardware) is the near-term transport; Pi headless daemon mode is a later
     horizon reusing the same recording features.
  3. **Mission amended** — MISSION.md gains a Positioning section (car-side complement to
     desk-side tuners; interop over duplication; bin editing/emulation/PROM permanent non-goals).

## Files written

- `product-knowledge/MISSION.md` — Positioning section added; header updated.
- `product-knowledge/ROADMAP.md` — positioning statement added; old "Phase 4: Beyond parity"
  restructured into **Horizon 1: Data prep for tuning** (XDF-aware correction export,
  suggested-change report, session report, cross-session diff), **Horizon 2: Onboard logging —
  bridge first** (TCPProvider, serve adapter, phone dash, headless record mode, MCU fallback),
  **Horizon 3: Interop & breadth** (ADX import, log format interop, config persistence,
  more ECMs). Permanent non-goals extended with bin editing / emulation / PROM burning.
  All prior Phase-4 items carried into a horizon (nothing dropped).
- `product-knowledge/TECH_STACK.md` — created (none existed): current stack recorded from the
  codebase; horizon additions all stdlib-first (encoding/xml for XDF/ADX, net for TCP,
  net/http for serve); non-stack list.
- `product-knowledge/PROJECT_STATUS.md` — Architecture notes TECH_STACK.md; Current Focus gains
  the Horizon-1 epic; Recent Changes entry added.

**Sequencing note**: Horizon 1 starts after the in-flight work closes out (TUI UX pass Phase D
merge + Phase E decision; Phase C Slice 2 deferred). TCPProvider (Horizon 2) unblocks on ESP32-S3
arrival independently of Horizon 1.

## Next steps

- `plan-feature` for the XDF-aware correction export (Horizon 1 lead feature) when ready.
- Resume TCPProvider implementation on ESP32-S3 arrival (existing spec).

