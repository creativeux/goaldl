<!--
GLaDOS-MANAGED STANDARD
Last Updated: 2026-07-04
-->
# Front-ends consume the Session API, never the pipeline internals

**Rule**: Every front-end (TUI, streaming monitor, future HTTP/WebSocket `serve`, mobile bridge) MUST consume `pkg/stream.Session` → `Snapshot`. A consumer never touches `pkg/decoder` or `pkg/ecm` directly. `Snapshot` fields MUST stay plain serializable data — no UI types, no callbacks, no unexported handles.

```go
sess := stream.NewSession(provider, registry, "1227747", promID)
err := sess.Run(ctx, func(snap stream.Snapshot) { /* render/serve/log */ })
```

Layering rules that follow:
- Frame-layout knowledge (offsets, flag bits, unit conversions) lives in `pkg/ecm` only.
- `pkg/blm` stays a generic RPM×MAP grid accumulator — no ECM specifics.
- Terminal rendering (`SensorTable`, `BLMBody`, `Renderer`, `BLMView`) is presentation on top of the core data path, shared by monitor and TUI, never inside it.
- New scripting commands are top-level words dispatched in `cmd/goaldl/main.go`, taking `args []string`; the bare command falls through to the TUI dashboard.

**Why**: The Session facade is the deliberate seam that lets one validated engine drive multiple faces (2026-07-03 refactor). Bypassing it re-couples front-ends to frame-layout details and forks the decode path — the exact accretion the consolidation deleted.
