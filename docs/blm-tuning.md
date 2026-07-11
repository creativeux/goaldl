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

## Straight into TunerPro: `-xdf`

Editing the tune in TunerPro? Point `blm` at the same XDF definition and it
builds the grid **on your VE table's own axes** and emits a paste block, so a
logged drive becomes applied corrections in one paste — no manual re-binning:

```bash
# What tables does the definition offer?
goaldl blm drive_4800.raw -xdf 42.xdf

# Build the correction on the Main VE Table's axes and export a paste block
goaldl blm drive_4800.raw -xdf 42.xdf -table "main ve" -paste ve.txt
```

In TunerPro open the VE table, select all cells, and use paste-with-multiply
(`Edit → Paste Special`): every cell is scaled by its correction factor.
Cells below the sample threshold export `1.000`, so unproven areas of the map
are left untouched. Samples beyond the table's axis range bin into the edge
cells (the command notes how many); clean WOT/decel frames were never recorded
in the first place.

Both XDF formats work — the legacy text format (tunerpro.net's own `$42`
definition) and the XML format modern community definitions use. Table titles
match case-insensitively and by substring; an ambiguous or unknown title
prints the candidates. Definitions whose axis breakpoints live in the bin
(rather than as literal labels) are refused with an explanation — goaldl
never reads the bin itself.
