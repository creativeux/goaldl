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
				Description: "Coolant temperature",
			},
			{
				Name:        "vehicle_speed",
				Offset:      5,
				Size:        1,
				Unit:        "MPH",
				Factor:      1,
				Description: "Vehicle speed",
			},
			{
				Name:        "map_voltage",
				Offset:      6,
				Size:        1,
				Unit:        "V",
				Factor:      0.0196,
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
	}
}
