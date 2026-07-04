<!--
GLaDOS-MANAGED STANDARD
Last Updated: 2026-07-04
-->
# Keep raw data raw — no filtering in the decode path

**Rule**: The decoder is a faithful transport. The decode path (`pkg/decoder`, frame emission in `pkg/stream`) MUST NOT contain plausibility filtering, outlier rejection, or smoothing. Emit every structurally-aligned frame warts-and-all. Quality signals ride *alongside* the data as fields (e.g. `PROMOK`, `ParseOK`, `Stats.FramesAborted`) — never as gates that drop frames.

```go
// Correct: quality is a field the consumer can act on.
snap := Snapshot{FrameEvent: ev, PROMOK: prom == s.promID, Sensors: vals}

// WRONG: silently dropping "implausible" data in the transport layer.
if coolantF > 250 { continue } // the drive capture's 221°F spike is intentionally preserved
```

**Why**: Data-quality decisions belong to downstream consumers/visualization, where thresholds are tunable and visible. Filtering in the transport hides sensor faults, wiring problems, and decoder bugs alike — exactly the signals a diagnostic tool exists to surface. The committed drive fixture deliberately preserves one 221°F coolant spike and three tail 0V-battery frames as a guard against regression on this rule.
