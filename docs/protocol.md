# How it works

## The ALDL 160-baud stream

The ALDL line idles high and encodes each bit as the *width* of a low pulse
(short ≈ logic 0, long ≈ logic 1). An inverting interface cable feeds this to a
PC UART, which frames exactly **one byte per ALDL bit** — so decoding is a
matter of reading byte *values*, not host-side timing (which USB makes
unreliable). At 4800 baud a short pulse arrives as `0xFE` and a long pulse as
`0x00`; nine consecutive 1-bits are the `0x1FF` sync that delimits 20-byte
frames.

That byte-value insight is the whole trick: earlier attempts (across many
tools, over months) failed because they treated the byte stream as a *timing*
record and tried to reconstruct pulse widths from it. The captured data was
clean the entire time — it just had to be read as values. The full model, the
frame layout for the GM 1227747, and the history of getting here are in
[`../CLAUDE.md`](../CLAUDE.md); the implementation is in `pkg/decoder/`.

## Data policy

The decoder is a faithful transport: it does **no** plausibility filtering,
outlier rejection, or smoothing, and emits every structurally-aligned frame
as-is (warts included). Quality signals ride alongside the data (e.g. PROM-ID
match); data-quality decisions belong to downstream consumers where they can be
tuned or disabled.

## References

- ALDL 160-baud spec: <https://www.techedge.com.au/vehicle/aldl160/160serial.htm>
- Decoding GM ALDL with a Teensy: <https://www.bot-thoughts.com/2018/01/decoding-gms-aldl-with-teensy-36.html>
- A033.ads ECM definition: `data/A033.ads`
