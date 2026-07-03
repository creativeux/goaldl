# Historical / superseded notes

These documents record the long debugging effort to decode 160-baud ALDL
before the working decoder existed. **They are kept for historical context
only — their technical conclusions are wrong and were superseded on
2026-07-03.** Each file carries a banner at the top explaining what it got
wrong.

For the correct protocol model and current architecture, see the top-level
[`CLAUDE.md`](../../CLAUDE.md) and the code in `pkg/decoder/`.

| File | What it is | Why it's superseded |
|------|-----------|---------------------|
| [`DECODER_STATUS.md`](DECODER_STATUS.md) | Status of the edge-timing decoder attempt | Blamed a macOS PL2303 "driver timing" issue; the real bug was the byteCount×208μs timing model. The captured signal was clean all along. |
| [`HARDWARE_DECODING.md`](HARDWARE_DECODING.md) | Protocol analysis and UART-sampling theories | Per-byte ones-counting and pulse-width-from-byte-count math are wrong; the correct model is one UART byte per ALDL bit, decoded from byte values. |

The one-paragraph version of how it was actually solved lives in `CLAUDE.md`
under "History — why past sessions failed."
