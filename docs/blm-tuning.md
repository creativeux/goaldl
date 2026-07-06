# BLM fuel-trim tuning

The `blm` command turns a drive capture into a fuel-trim map — a picture of
where the base tune runs rich or lean across RPM and load. It reads the Block
Learn Multiplier (BLM, the ECM's long-term fuel trim) from each frame and bins
it into an RPM × MAP grid.

**Reading BLM: 128 is neutral.** Below 128 the ECM is *removing* fuel because
the mixture ran rich (the base tune has too much fuel there); above 128 it is
*adding* fuel because it ran lean. The **correction factor** each cell reports
is `avg_BLM / 128` — multiply that cell's base VE/fuel by it to move the ECM
back toward 128.

Only closed-loop, block-learn-enabled frames are recorded — BLM is frozen and
meaningless at wide-open throttle, on decel, or before warm-up, so those frames
are skipped. A cell also isn't trusted until it has collected enough readings
(default 4; BLM hunts, so one or two samples are noisy). Below that threshold a
cell's correction is held at `1.000` (no change) and, in the live view, drawn
dim while it accumulates.

```bash
# Offline: build the tables from a capture, write the correction grid to CSV
goaldl blm drive_4800.raw -o correction.csv
goaldl blm drive_4800.raw -min 3   # trust a cell at 3 samples (WinALDL-like)

# Live: watch each cell fill and settle as you drive — in the dashboard's BLM
# tab, or the streaming grid (· = empty, dim = accumulating, solid = trusted)
goaldl -p /dev/cu.usbserial-10                              # dashboard, press 2
goaldl monitor -p /dev/cu.usbserial-10 -blm -o session.raw   # streaming grid
```

`blm` prints three tables — Samples, Wide Average BLM, and the Correction
factor — matching the format of `data/20250601_162123_BLM.txt`. The MAP→kPa
axis uses a standard GM 1-bar transfer (`pkg/ecm/fueltrim.go`); it only affects
which column a reading lands in, not the BLM math.
