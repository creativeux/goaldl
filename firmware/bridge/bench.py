# Bench tool — NOT the firmware. Copy to CIRCUITPY alongside code.py and run
# from the REPL:  import bench
#
# Prints the raw logic level on the UART RX pin ~2x/second. Two jobs:
#
#   1. Static verification of the input-conditioning stage (README "Input
#      conditioning") with no signal source at all. Jumper the stage's input
#      (ALDL pin E side of R1) to 3V and to GND in turn; RX must read the
#      OPPOSITE each time -- the stage inverts. Same level both ways, or
#      inverted from the table below, means Q1 is in backwards.
#
#         stage input -> 3V   =>  RX LOW
#         stage input -> GND  =>  RX HIGH
#         stage input floating =>  RX HIGH (with R3 fitted)
#
#   2. First diagnostic in the car: if goaldl decodes nothing, this answers
#      "is the RX pin even toggling?" before anyone suspects the decoder.
#      A live ALDL line makes it flicker constantly.
#
# code.py owns board.RX as a UART, so only one of the two can run at a time.
# Ctrl-C out (or reset) before letting code.py take the pin back.

import time

import board
import digitalio

pin = digitalio.DigitalInOut(board.RX)
pin.direction = digitalio.Direction.INPUT

print("bench: reading board.RX -- Ctrl-C to stop")
while True:
    print("RX =", "HIGH" if pin.value else "LOW")
    time.sleep(0.5)
