# Observed Philosophies

*This file is populated automatically by the `pattern-observer` module during normal workflow execution.*
*Items here are reviewed and promoted (or discarded) during `{{CMD}}recombobulate`.*

---

<!-- Add observations below this line -->

### 2026-07-04 - Operating state that gates a view belongs in persistent chrome
- **Source**: User correction during spec-feature (Phase 2). On seeing the INT/O2/BLM grid design, the user said loop state "should be a persistent status across all tabs (and determines whether those tabs have any value)".
- **Context**: Loop state (Open/Closed) was originally rendered only inside the BLM tab's body, yet it governs whether the BLM and INT grids accumulate at all. Promoted to a fixed loop-status line under the tab bar, on every tab (`stream.LoopStatus`).
- **Proposed Philosophy**: "If a piece of ECM/operating state determines whether a view is meaningful right now, surface it as persistent chrome — not buried inside the view it gates. The operator should never have to switch tabs to learn that the tab they left was showing frozen data."
- **Suggested Weight**: preferred
- **Suggested Domain**: ux
- **Confidence**: Medium
- **Status**: pending
