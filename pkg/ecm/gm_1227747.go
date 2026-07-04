package ecm

// gm1227747 returns the ECM definition for GM 1227747 (A033 TBI ECM).
//
// Each parameter's conversion is expressed as data (Factor/Bias, or a Lookup),
// per A033.ads. To add a sensor or change a scale, edit this table only — the
// parser (extractParameterValue) is generic and never changes.
func gm1227747() Definition {
	return Definition{
		PartNumber:   "1227747",
		Description:  "GM A033 TBI ECM (86-88 4.3/5.0/5.7L)",
		DataStreamID: 0xF1,
		FrameSize:    20,
		Parameters: []Parameter{
			{
				Name:        "prom_id",
				Offset:      1,
				Size:        2, // 16-bit big-endian ID
				Factor:      1,
				Description: "PROM ID (16-bit)",
			},
			{
				Name:        "iac_position",
				Offset:      3,
				Size:        1,
				Unit:        "steps",
				Factor:      1,
				Description: "IAC position",
			},
			{
				Name:        "coolant_temp",
				Offset:      4,
				Size:        1,
				Unit:        "°F",
				Lookup:      coolantTempLookup,
				Alt:         &AltConversion{Lookup: coolantTempCelsius, Unit: "°C"},
				Description: "Coolant temperature",
			},
			{
				Name:        "vehicle_speed",
				Offset:      5,
				Size:        1,
				Unit:        "MPH",
				Factor:      1,
				Alt:         &AltConversion{Factor: 1.609, Unit: "KPH"},
				Description: "Vehicle speed",
			},
			{
				Name:   "map_voltage",
				Offset: 6,
				Size:   1,
				Unit:   "V",
				Factor: 0.0196,
				// kPa = (raw + 28.06) / 2.71 — VERIFIED against the WinALDL
				// log 2026-07-04 (exact over raw 49..190; see MapVoltsToKPa).
				Alt:         &AltConversion{Factor: 1 / 2.71, Bias: 28.06 / 2.71, Unit: "kPa"},
				Description: "MAP sensor voltage",
			},
			{
				Name:        "engine_rpm",
				Offset:      7,
				Size:        1,
				Unit:        "RPM",
				Factor:      25,
				Description: "Engine speed",
			},
			{
				Name:        "tps_voltage",
				Offset:      8,
				Size:        1,
				Unit:        "V",
				Factor:      0.0196,
				Alt:         TPSPercentAlt(DefaultTPS0, DefaultTPS100),
				Description: "TPS voltage",
			},
			{
				Name:        "integrator",
				Offset:      9,
				Size:        1,
				Unit:        "counts",
				Factor:      1,
				Description: "Integrator (short-term fuel trim)",
			},
			{
				Name:        "oxygen_sensor",
				Offset:      10,
				Size:        1,
				Unit:        "mV",
				Factor:      4.44,
				Description: "O2 sensor voltage",
			},
			{
				Name:        "battery_voltage",
				Offset:      15,
				Size:        1,
				Unit:        "V",
				Factor:      0.1,
				Description: "Battery voltage",
			},
			{
				Name:        "knock_count",
				Offset:      17,
				Size:        1,
				Unit:        "counts",
				Factor:      1,
				Description: "Knock counter",
			},
			{
				Name:        "blm",
				Offset:      18,
				Size:        1,
				Unit:        "counts",
				Factor:      1,
				Description: "Block Learn Multiplier",
			},
			{
				Name:        "rich_lean_counter",
				Offset:      19,
				Size:        1,
				Unit:        "transitions",
				Factor:      1,
				Description: "Rich/Lean transition counter",
			},
		},

		// Per-byte labels for the raw-stream view, WinALDL naming.
		ByteLabels: []string{
			"MW2", "PROMIDA", "PROMIDB", "IAC", "CT", "MPH", "MAP", "RPM",
			"TPS", "INT", "O2", "MALFFLG1", "MALFFLG2", "MALFFLG3", "MWAF1",
			"VOLT", "MCU2IO", "KNOCK_CNT", "BLM", "O2_CNT",
		},

		// Status-word bits per A033.ads (bit order verified against the
		// WinALDL log's per-bit columns and live rows — see flags.go).
		FlagWords: []FlagWord{
			{Name: "MW2", Offset: 0, Bits: []FlagBit{
				{Bit: 0, Name: "VSS pulse occurred"},
				{Bit: 1, Name: "Code 43 ready for 2nd test"},
				{Bit: 2, Name: "Reference pulse occurred"},
				{Bit: 3, Name: "Diag switch: factory test (3.9 kΩ)"},
				{Bit: 4, Name: "Diag switch: field test (shorted)"},
				{Bit: 5, Name: "Diag switch: ALDL (10 kΩ)"},
				{Bit: 6, Name: "Battery voltage high"},
				{Bit: 7, Name: "Idle flag", SetLabel: "IDLE"},
			}},
			{Name: "MWAF1", Offset: 14, Bits: []FlagBit{
				{Bit: 0, Name: "Cranked in clear flood"},
				{Bit: 1, Name: "Block learn enable"},
				{Bit: 2, Name: "Battery voltage low (IAC inhibited)"},
				{Bit: 3, Name: "4-3 downshift for TCC unlock"},
				{Bit: 4, Name: "Async fuel"},
				{Bit: 5, Name: "High gear last pass"},
				{Bit: 6, Name: "Rich/lean", SetLabel: "RICH"},
				{Bit: 7, Name: "Loop status", SetLabel: "CLOSED"},
			}},
			// Bit 2 (A/C disabled) is absent from A033.ads; taken from
			// WinALDL. Bit 7's negated sense ("no A/C requested") matches
			// live log data: set at idle with A/C off. Bit 6 is undefined.
			{Name: "MCU2IO", Offset: 16, Bits: []FlagBit{
				{Bit: 0, Name: "AIR switch solenoid", SetLabel: "ON"},
				{Bit: 1, Name: "AIR divert solenoid", SetLabel: "ON"},
				{Bit: 2, Name: "A/C disabled"},
				{Bit: 3, Name: "TCC", SetLabel: "LOCKED"},
				{Bit: 4, Name: "Park/Neutral"},
				{Bit: 5, Name: "High gear"},
				{Bit: 7, Name: "No A/C requested"},
			}},
		},

		// Trouble codes per A033.ads MALFFLG1-3 (offsets 11-13). Codes the
		// ADS marks N/A for this application carry Unused (WinALDL's generic
		// labels kept as description where the ADS is silent).
		ErrorCodes: []ErrorCode{
			{Code: 12, Description: "No reference pulses (engine not running)", Offset: 11, Bit: 7},
			{Code: 13, Description: "O2 sensor open", Offset: 11, Bit: 6},
			{Code: 14, Description: "Coolant temp high", Offset: 11, Bit: 5},
			{Code: 15, Description: "Coolant temp low", Offset: 11, Bit: 4},
			{Code: 21, Description: "TPS high", Offset: 11, Bit: 3},
			{Code: 22, Description: "TPS low", Offset: 11, Bit: 2},
			{Code: 23, Description: "IAT/MAT low", Offset: 11, Bit: 1, Unused: true},
			{Code: 24, Description: "VSS (vehicle speed sensor)", Offset: 11, Bit: 0},
			{Code: 25, Description: "IAT/MAT high", Offset: 12, Bit: 7, Unused: true},
			{Code: 31, Description: "Governor fail", Offset: 12, Bit: 6, Unused: true},
			{Code: 32, Description: "EGR", Offset: 12, Bit: 5},
			{Code: 33, Description: "MAP high", Offset: 12, Bit: 4},
			{Code: 34, Description: "MAP low", Offset: 12, Bit: 3},
			{Code: 35, Description: "IAC", Offset: 12, Bit: 2},
			{Code: 41, Description: "Not used", Offset: 12, Bit: 1, Unused: true},
			{Code: 42, Description: "EST monitor", Offset: 12, Bit: 0},
			{Code: 43, Description: "ESC (knock)", Offset: 13, Bit: 7},
			{Code: 44, Description: "O2 lean", Offset: 13, Bit: 6},
			{Code: 45, Description: "O2 rich", Offset: 13, Bit: 5},
			{Code: 51, Description: "PROM error", Offset: 13, Bit: 4},
			{Code: 52, Description: "CAL-PACK missing", Offset: 13, Bit: 3},
			{Code: 53, Description: "Not used", Offset: 13, Bit: 2, Unused: true},
			{Code: 54, Description: "Fuel pump relay", Offset: 13, Bit: 1},
			{Code: 55, Description: "A/D unit", Offset: 13, Bit: 0},
		},
	}
}

// coolantTempCelsius is the metric alternate of the coolant thermistor curve.
// (WinALDL's own °F curve is smooth and reads ~3°F below this stepped A033
// table at warm idle — accepted divergence; the ADS table is our authority.)
func coolantTempCelsius(raw byte) float64 {
	return (coolantTempLookup(raw) - 32) / 1.8
}
