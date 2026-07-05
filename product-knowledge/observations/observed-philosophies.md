# Observed Philosophies

*This file is populated automatically by the `pattern-observer` module during normal workflow execution.*
*Items here are reviewed and promoted (or discarded) during `{{CMD}}recombobulate`.*

---

<!-- Add observations below this line -->

### 2026-07-04 - Annotate known-suspect data loudly; never hide or filter it in the view
- **Source**: Repeated pattern (3+ applications) + user decision in Phase A spec Q&A. When shown the free-running-knock finding, the user chose "warn + keep the grid at normal brightness" over dimming or suppressing the values.
- **Context**: The consumer-side corollary of `standards/decoder/raw-data-policy.md` (which governs the *decode* path) has now been applied at the *view* layer three times: (1) Phase 1 bad-sample gating renders the raw tab from every frame while decoded tabs hold the last good one; (2) Phase A's free-running-counter warning annotates the Spark grid but leaves its artifact values fully visible; (3) Phase A's staleness flag marks data old without removing it. In every case the suspect data stays on screen unchanged; only an adjacent signal (status line, heartbeat glyph, footer) tells the operator to distrust it.
- **Proposed Philosophy**: "The raw-data-raw policy has a view-layer twin: when displayed data is known-suspect (stale, an artifact, a bad sample), the view annotates it prominently rather than hiding, dimming into illegibility, or filtering it. A diagnostic tool that quietly drops or masks bad readings hides exactly the faults it exists to surface — so the fix is always more signal, never less data."
- **Suggested Weight**: preferred
- **Suggested Domain**: ux
- **Confidence**: High
- **Status**: pending

### 2026-07-04 - Operating state that gates a view belongs in persistent chrome
- **Source**: User correction during spec-feature (Phase 2). On seeing the INT/O2/BLM grid design, the user said loop state "should be a persistent status across all tabs (and determines whether those tabs have any value)".
- **Context**: Loop state (Open/Closed) was originally rendered only inside the BLM tab's body, yet it governs whether the BLM and INT grids accumulate at all. Promoted to a fixed loop-status line under the tab bar, on every tab (`stream.LoopStatus`).
- **Proposed Philosophy**: "If a piece of ECM/operating state determines whether a view is meaningful right now, surface it as persistent chrome — not buried inside the view it gates. The operator should never have to switch tabs to learn that the tab they left was showing frozen data."
- **Suggested Weight**: preferred
- **Suggested Domain**: ux
- **Confidence**: Medium
- **Status**: pending
