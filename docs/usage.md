# Usage

> The examples below write the command as `goaldl` for brevity. Run it however
> suits you — `./goaldl` from its folder, `go run ./cmd/goaldl` from a source
> checkout, or `goaldl` if it's on your PATH. See [Install](../README.md#install).

`goaldl` is two things in one binary:

- **The dashboard** — the interactive terminal UI you get by running `goaldl`
  with no command word. This is the main way to use it.
- **Scripting commands** — `record`, `decode`, `monitor`, `blm`, `simulate`,
  `ports`, `ecms`. A recognised command word as the first argument runs that
  instead of the dashboard.

## The dashboard

```bash
goaldl -p /dev/cu.usbserial-10   # live from the car (find the port with: goaldl ports)
goaldl                           # auto-connects if exactly one USB serial port is present
goaldl drive_4800.raw            # replay a capture file (-speed N to scrub)
```

It opens a set of tabs across the top:

- **Sensors** — the live sensor table (RPM, coolant, MAP, TPS, O2, battery, …),
  each with a running min/max. TPS reads as a percentage; calibrate it with
  `-tps0` / `-tps100` (the raw closed- and wide-open values for your throttle).
- **BLM / INT / O2** — the fuel-trim grids (see
  [BLM fuel-trim tuning](blm-tuning.md)).
- **Spark** — knock/spark counts across RPM × load.
- **Flags** and **Codes** — ECM status words and stored trouble codes.
- **Raw** — a scrolling view of the raw frame bytes.

**Keys:** number keys or `tab` switch tabs · `q` quits. In-session actions
(each opens a filename prompt; nothing is ever overwritten):

- `s` — save the fuel-trim grids to text files.
- `c` — clear the active grid, or reset the sensor min/max.
- `r` — start/stop recording the raw byte stream to a file.
- `d` — start/stop logging decoded frames to a CSV.
- `space` / `+` / `-` — pause and change replay speed (0.25×–16×; live is
  unaffected).

## The workflow: record once, work offline

The recommended pattern is to **capture raw bytes once at the car, then work
from the file** as many times as you like:

```bash
# 1. At the car — find the adapter and record a drive
goaldl ports                                             # port name drifts; check it
goaldl record -p /dev/cu.usbserial-10 -t 600 -o drive.raw   # 10-minute capture

# 2. Later, at your desk — replay or analyse the file
goaldl drive.raw                     # replay in the dashboard
goaldl decode drive.raw -o frames.csv   # decode every frame to CSV
goaldl blm drive.raw -o correction.csv  # build the fuel-trim map
```

## Scripting commands

```bash
goaldl ports                        # list USB serial ports (find your adapter)
goaldl ecms                         # list the ECM definitions goaldl knows

goaldl record  -p <port> -t 60 -o session.raw       # capture raw bytes to a file
goaldl decode  session.raw -o frames.csv            # batch-decode a capture to CSV
goaldl monitor -p <port> -o session.raw -csv live.csv   # streaming sensor table (non-interactive)
goaldl monitor session.raw                          # replay a capture as a streaming table
goaldl blm     session.raw -o correction.csv        # build the BLM fuel-trim table
goaldl monitor -p <port> -blm -o session.raw        # live streaming BLM grid

goaldl simulate -n 10 && goaldl decode aldl_sim_4800.raw   # synthetic data, no hardware
goaldl version                      # print the build version
goaldl help                         # full usage
```

## Hardware & drivers

You need:

- **A compatible GM vehicle** with an ALDL port — usually a 12-pin connector
  under the dash.
- **A USB-to-ALDL cable** — an inverting level converter onto the UART RX line.
  A Prolific **PL2303** or a genuine FTDI **FT232R** both work.

For the driver each OS needs (and the supported-platform matrix), see
[Platform support](../README.md#platform-support).

## References

- ALDL 160-baud spec: <https://www.techedge.com.au/vehicle/aldl160/160serial.htm>
- Decoding GM ALDL with a Teensy: <https://www.bot-thoughts.com/2018/01/decoding-gms-aldl-with-teensy-36.html>
- A033.ads ECM definition: `data/A033.ads`
