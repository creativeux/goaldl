# Changelog

## [0.1.3](https://github.com/creativeux/goaldl/compare/v0.1.2...v0.1.3) (2026-07-19)


### Features

* ESP32-S3 ALDL bridge firmware (CircuitPython UART→TCP) ([#43](https://github.com/creativeux/goaldl/issues/43)) ([5a644f5](https://github.com/creativeux/goaldl/commit/5a644f5e8f05f5b6493d5389cbcc3dcc9fa71cd7))
* TCPProvider — consume the ALDL byte stream over a TCP bridge (-tcp) ([#42](https://github.com/creativeux/goaldl/issues/42)) ([29d5d1c](https://github.com/creativeux/goaldl/commit/29d5d1c856978104cb97970bf821b84849e1152a))
* XDF-aware correction export for TunerPro paste ([#41](https://github.com/creativeux/goaldl/issues/41)) ([369083d](https://github.com/creativeux/goaldl/commit/369083d6718140bd8fe94a57e4c9fa3236db6087))


### Bug Fixes

* publish verdict from a comment marker in a deterministic step ([#32](https://github.com/creativeux/goaldl/issues/32)) ([11be72c](https://github.com/creativeux/goaldl/commit/11be72c8e036bce2dc3441ad19cfeb298d1c103d))
* review agent publishes its own commit status ([#29](https://github.com/creativeux/goaldl/issues/29)) ([da0d800](https://github.com/creativeux/goaldl/commit/da0d800fbf2e4c4e3111cf2322cb88d9a9d2db1c))
* stop the agent's own comment from cancelling its review run ([#35](https://github.com/creativeux/goaldl/issues/35)) ([2be2ad4](https://github.com/creativeux/goaldl/commit/2be2ad4bd027395eef62438829993413e653f791))

## [0.1.2](https://github.com/creativeux/goaldl/compare/v0.1.1...v0.1.2) (2026-07-06)


### Features

* ship linux/armv6 and FreeBSD release binaries ([08cf396](https://github.com/creativeux/goaldl/commit/08cf396cf8d9400ead1706b404ea4291beb5d029))


### Bug Fixes

* detect CH340 adapters and BSD ports in the serial port list ([6f64041](https://github.com/creativeux/goaldl/commit/6f64041d0b8fa2b182a4fcfe150504e64caaa683))
* enable virtual-terminal mode before ANSI redraws on Windows ([b6193f7](https://github.com/creativeux/goaldl/commit/b6193f730ec52b9d9b22608f76ddbf174b34845c))
* exclude unintended freebsd/arm64 from the release matrix ([441d264](https://github.com/creativeux/goaldl/commit/441d2646ae7788f9ab736032f84f2686c37b1727))

## [0.1.1](https://github.com/creativeux/goaldl/compare/v0.1.0...v0.1.1) (2026-07-06)


### Features

* add replay seek/position, byte diagnostics, and port picker to the TUI ([ed5800f](https://github.com/creativeux/goaldl/commit/ed5800ff0fbd206995d1d94fb8521c495bde26dd))
* show the build version next to the GoALDL title in the dashboard ([4cdd81d](https://github.com/creativeux/goaldl/commit/4cdd81df2e43968702d9d4a188c35e35eaeb2cfc))


### Bug Fixes

* keep the live session alive across a dropped or absent serial port ([1a0e496](https://github.com/creativeux/goaldl/commit/1a0e496b62dc0580c0aaeff1d5dd66ff953fafbe))

## 0.1.0 (2026-07-06)


### Features

* add versioning, conventional-commit releases, and cross-platform binaries ([#8](https://github.com/creativeux/goaldl/issues/8)) ([c9ea02e](https://github.com/creativeux/goaldl/commit/c9ea02e6e031f2c719f2ddd97fbf0a4a7078d455))
