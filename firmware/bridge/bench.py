# Bench tool — NOT the firmware. Copy to CIRCUITPY alongside code.py and run
# from the REPL:  import bench
#
# Prints the raw logic level on the UART RX pin ~2x/second. Two jobs:
#
#   1. Static verification of the input-conditioning stage (README "Input
#      conditioning") with no signal source at all. Jumper the stage's input
#      (ALDL pin E side of R1) to 3V and to GND in turn. The two-stage build
#      does NOT invert -- RX follows the input:
#
#         stage input -> 3V    =>  RX HIGH
#         stage input -> GND   =>  RX LOW
#         stage input floating =>  RX LOW (with R3 fitted)
#
#      Same level both ways means a stage isn't switching (check the 2N3904
#      markings -- a 2N3906 is a PNP in the same package -- the emitter
#      grounds, and the base resistors). The OPPOSITE table means only one
#      stage is in circuit: most likely RX is still wired to Q1's collector
#      instead of Q2's. A one-stage build cannot sync; see the README's byte-
#      mix table.
#
#   2. First diagnostic in the car: if goaldl decodes nothing, this answers
#      "is the RX pin even toggling?" before anyone suspects the decoder.
#      Expect MOSTLY HIGH with an occasional LOW -- the line's low duty cycle
#      is only ~13% (189 bits per 1.18 s frame, each 6250 us cell carrying a
#      365 or 1875 us low pulse), and this samples asynchronously at 2 Hz, so
#      roughly one print in eight lands inside a pulse. Watch 20-30 samples:
#      all HIGH across that span means nothing is arriving.
#
# code.py owns board.RX as a UART, so only one of the two can run at a time.
# The finally: below releases the pin on Ctrl-C, so code.py can reclaim it --
# without that, a DigitalInOut left holding board.RX makes the next
# busio.UART() raise "RX in use" until a soft reboot.

import time

import board
import digitalio

pin = digitalio.DigitalInOut(board.RX)
pin.direction = digitalio.Direction.INPUT

print("bench: reading board.RX -- Ctrl-C to stop")
try:
    while True:
        print("RX =", "HIGH" if pin.value else "LOW")
        time.sleep(0.5)
finally:
    pin.deinit()
