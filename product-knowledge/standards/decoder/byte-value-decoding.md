<!--
GLaDOS-MANAGED STANDARD
Last Updated: 2026-07-04
-->
# Decode from byte values, never host-side timing

**Rule**: Any code interpreting the ALDL byte stream MUST derive bits from byte *values* (trailing-zero counts fixed by the adapter's hardware UART). It MUST NOT use host timestamps, inter-byte arrival times, or byte counts as a proxy for pulse duration.

```go
// Correct: the byte value itself measures the pulse width.
bit, clean := cfg.ClassifyByte(b) // 0xFE → 0, 0x00 → 1 at 4800 baud

// WRONG: reconstructing timing from the host side.
pulseMicros := float64(byteCount) * 208.0 // idle gaps produce no bytes at all
```

**Why**: The UART frames exactly one byte per ALDL bit; USB/driver buffering only delays delivery (16ms-class latency vs 365μs pulses) but never changes the values. Months of past debugging failed because every decoder treated the stream as a timing record — the full post-mortem is in CLAUDE.md ("History — why past sessions failed"). Sync is 9 consecutive `0x00` bytes at 4800 baud, not a measured 1872μs pulse.

**Classification**: use one coarse threshold (~1100μs equivalent) between short and long pulses, never tight per-ECM ranges — pulse widths vary by ECM family (spec ~1500–4400μs for logic 1).
