# goaldl ALDL bridge — ESP32-S3 (Adafruit QT Py ESP32-S3), CircuitPython 10.x
#
# Forwards the raw ALDL UART byte stream (4800 baud, one UART byte per ALDL
# bit — see goaldl's pkg/decoder) to a single TCP client. goaldl consumes it
# with:  goaldl -tcp 192.168.4.1:3333
#
# The bridge does ZERO protocol work: no framing, no filtering, no timing.
# It is a byte pipe; goaldl's decoder finds frame sync itself (raw-data policy).
#
# Config via settings.toml (all optional):
#   BRIDGE_MODE     = "ap" (default) or "sta"
#   BRIDGE_SSID     = AP name to create, or network to join   (default "goaldl")
#   BRIDGE_PASSWORD = WPA2 password                            (default "aldl1227")
#   BRIDGE_PORT     = TCP listen port                          (default 3333)
#   BRIDGE_TEST     = 1 to replay synthetic ALDL frames instead of reading the
#                     UART — validates the WiFi/TCP path with no wiring at all.
#
# Wiring (real mode): ALDL-side TTL serial TX -> board RX pin, GND -> GND.
# 3.3V logic only. The UART idles high; the interface cable's inversion means
# bytes arrive as 0xFE (short pulse / logic 0) and 0x00 (long pulse / logic 1).
#
# Status LED (NeoPixel): yellow = starting · blue = up, waiting for a client ·
# green = client connected · red blink = client dropped.

import time

import board
import busio
import os
import socketpool
import wifi

MODE = os.getenv("BRIDGE_MODE", "ap")
SSID = os.getenv("BRIDGE_SSID", "goaldl")
PASSWORD = os.getenv("BRIDGE_PASSWORD", "aldl1227")
PORT = int(os.getenv("BRIDGE_PORT", "3333"))
TEST = int(os.getenv("BRIDGE_TEST", "0"))

BAUD = 4800

# --- status LED (best-effort; firmware must run without the neopixel lib) ----
try:
    import neopixel

    _pixel = neopixel.NeoPixel(board.NEOPIXEL, 1, brightness=0.15)
except Exception:
    _pixel = None


def status(color):
    if _pixel is not None:
        _pixel[0] = color


YELLOW, BLUE, GREEN, RED, OFF = (
    (255, 180, 0),
    (0, 60, 255),
    (0, 255, 40),
    (255, 0, 0),
    (0, 0, 0),
)

# --- synthetic ALDL source (BRIDGE_TEST=1) -----------------------------------
# Encodes GM-1227747-shaped frames exactly as a 4800-baud UART would see the
# 160-baud line: one byte per ALDL bit, 0xFE = logic 0, 0x00 = logic 1; a
# character is 1 mode bit (0) + 8 data bits MSB-first; sync = nine 1-bits.
# Paced to the real cadence (189 bits ≈ 1.18 s/frame ≈ 160 B/s).

_TEST_FRAME = bytes(
    [128, 24, 147, 40, 91, 0, 96, 24, 26, 128, 106, 0, 0, 0, 4, 138, 32, 0, 128, 77]
)


def _encode_frame(data):
    out = bytearray()
    for b in data:
        out.append(0xFE)  # mode bit 0 (data character)
        for i in range(7, -1, -1):
            out.append(0x00 if (b >> i) & 1 else 0xFE)
    out.extend(b"\x00" * 9)  # 0x1FF sync: nine consecutive 1-bits
    return bytes(out)


class TestSource:
    """Yields encoded test-frame bytes paced to the real ~160 B/s."""

    def __init__(self):
        self._stream = _encode_frame(_TEST_FRAME)
        self._pos = 0
        self._last = time.monotonic()

    def read(self, _n):
        now = time.monotonic()
        n = int((now - self._last) * 160)
        if n <= 0:
            return None
        # Advance by the bytes actually emitted (not to `now`) so truncated
        # fractions carry over — otherwise the effective rate sags well below
        # 160 B/s and the frame cadence reads slow in goaldl.
        self._last += n / 160.0
        chunk = bytearray()
        for _ in range(n):
            chunk.append(self._stream[self._pos])
            self._pos = (self._pos + 1) % len(self._stream)
        return bytes(chunk)


# --- byte source -------------------------------------------------------------
if TEST:
    source = TestSource()
    print("bridge: TEST mode — synthetic ALDL frames, no UART")
else:
    # timeout=0: read() returns whatever is buffered (or None), never blocks.
    source = busio.UART(
        board.TX, board.RX, baudrate=BAUD, timeout=0, receiver_buffer_size=4096
    )
    print("bridge: UART on RX pin @", BAUD, "baud")

# --- WiFi --------------------------------------------------------------------
status(YELLOW)
if MODE == "sta":
    print("bridge: joining", SSID, "...")
    wifi.radio.connect(SSID, PASSWORD)
    ip = wifi.radio.ipv4_address
else:
    print("bridge: starting AP", SSID)
    wifi.radio.start_ap(ssid=SSID, password=PASSWORD)
    ip = wifi.radio.ipv4_address_ap
    while ip is None:  # the AP takes a moment to come up
        time.sleep(0.2)
        ip = wifi.radio.ipv4_address_ap
print("bridge: listening on %s:%d" % (ip, PORT))

pool = socketpool.SocketPool(wifi.radio)
server = pool.socket(pool.AF_INET, pool.SOCK_STREAM)
server.setsockopt(pool.SOL_SOCKET, pool.SO_REUSEADDR, 1)
server.bind(("0.0.0.0", PORT))
server.listen(1)
server.setblocking(False)

# --- forward loop ------------------------------------------------------------
# Single client (goaldl). With no client the UART is still drained so a stale
# backlog is never delivered later. goaldl's provider redials on drop, so a
# dead client is simply closed and the next accept wins.
client = None
sent = 0
while True:
    if client is None:
        status(BLUE)
        try:
            client, addr = server.accept()
            client.setblocking(True)
            sent = 0
            print("bridge: client", addr)
            status(GREEN)
        except OSError:
            pass

    data = source.read(256)
    if data:
        if client is not None:
            try:
                client.send(data)
                sent += len(data)
            except OSError:
                print("bridge: client dropped after", sent, "bytes")
                try:
                    client.close()
                except OSError:
                    pass
                client = None
                status(RED)
                time.sleep(0.3)
    else:
        time.sleep(0.005)
