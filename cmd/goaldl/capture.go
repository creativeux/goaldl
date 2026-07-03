package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"goaldl/pkg/aldl"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/serial"
)

// cmdRecord captures raw UART bytes to a file with zero processing, so all
// decoder work can happen offline and repeatably. One 60s idle capture at the
// car is enough to develop against at a desk.
func cmdRecord() {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	portName := fs.String("p", "", "Serial port name (required)")
	baudRate := fs.Int("b", 4800, "UART sampling baud rate")
	output := fs.String("o", "", "Output file (default aldl_capture_<baud>.raw)")
	seconds := fs.Int("t", 60, "Capture duration in seconds (Ctrl+C stops early)")
	fs.Parse(os.Args[2:])

	if *portName == "" {
		fmt.Fprintln(os.Stderr, "Error: port name required (-p)")
		os.Exit(1)
	}
	outName := *output
	if outName == "" {
		outName = fmt.Sprintf("aldl_capture_%d.raw", *baudRate)
	}

	ser, err := serial.NewWithBaudRate(*portName, *baudRate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening port: %v\n", err)
		os.Exit(1)
	}
	defer ser.Close()

	f, err := os.Create(outName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if err := ser.ResetInputBuffer(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not flush input buffer: %v\n", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("Recording raw bytes from %s at %d baud for %ds (Ctrl+C to stop early)...\n",
		*portName, *baudRate, *seconds)
	fmt.Println("Expected rate for a live 160-baud ALDL stream: ~160 bytes/sec")

	var total int64
	histogram := make(map[byte]int64)
	buf := make([]byte, 512)
	start := time.Now()
	deadline := start.Add(time.Duration(*seconds) * time.Second)
	lastReport := start

capture:
	for time.Now().Before(deadline) {
		select {
		case <-sigCh:
			fmt.Println("\nInterrupted, finishing up...")
			break capture
		default:
		}

		n, err := ser.Read(buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nRead error: %v\n", err)
			break
		}
		if n == 0 {
			continue // read timeout, no data yet
		}
		if _, err := f.Write(buf[:n]); err != nil {
			fmt.Fprintf(os.Stderr, "\nWrite error: %v\n", err)
			os.Exit(1)
		}
		total += int64(n)
		for _, b := range buf[:n] {
			histogram[b]++
		}
		if time.Since(lastReport) >= 2*time.Second {
			elapsed := time.Since(start).Seconds()
			fmt.Printf("\r  %6d bytes, %5.1f bytes/sec", total, float64(total)/elapsed)
			lastReport = time.Now()
		}
	}

	elapsed := time.Since(start).Seconds()
	fmt.Printf("\nCaptured %d bytes in %.1fs (%.1f bytes/sec) to %s\n", total, elapsed, float64(total)/elapsed, outName)
	printHistogram(histogram, total)

	if total == 0 {
		fmt.Println("\nNo data received. Check that the engine is on, the cable is connected,")
		fmt.Println("and the port is correct (goaldl ports).")
		return
	}
	fmt.Printf("\nNext: goaldl decode %s -b %d\n", outName, *baudRate)
}

func printHistogram(histogram map[byte]int64, total int64) {
	if total == 0 {
		return
	}
	type entry struct {
		b byte
		n int64
	}
	entries := make([]entry, 0, len(histogram))
	for b, n := range histogram {
		entries = append(entries, entry{b, n})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].n > entries[j].n })

	fmt.Println("Byte value distribution (top 8):")
	for i, e := range entries {
		if i >= 8 {
			break
		}
		hint := ""
		switch {
		case e.b == 0x00:
			hint = "← logic 1 (long pulse)"
		case e.b == 0xFE || e.b == 0xFC || e.b == 0xFF:
			hint = "← logic 0 (short pulse)"
		}
		fmt.Printf("  0x%02X: %6d (%4.1f%%) %s\n", e.b, e.n, 100*float64(e.n)/float64(total), hint)
	}
}

// cmdDecode runs the byte-value decoder over a raw capture file (or live
// from a serial port with -p) and reports frames, PROM ID validation, and
// sensor values.
func cmdDecode() {
	fs := flag.NewFlagSet("decode", flag.ExitOnError)
	portName := fs.String("p", "", "Decode live from this serial port instead of a file")
	baudRate := fs.Int("b", 4800, "UART sampling baud rate the capture was recorded at")
	ecmPart := fs.String("e", defaultECM, "ECM part number")
	output := fs.String("o", "", "Write decoded frames to CSV file")
	promID := fs.Int("prom", 6291, "Expected PROM ID for frame validation (0 to disable)")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	verbose := fs.Bool("v", false, "Print every frame instead of the first 5")
	fs.Parse(os.Args[2:])

	if *portName != "" {
		liveDecode(*portName, *baudRate, *ecmPart, *output, *promID, *invert)
		return
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: goaldl decode <capture.raw> [-b baud] [-o out.csv], or goaldl decode -p <port>")
		os.Exit(1)
	}
	inName := fs.Arg(0)
	// The flag package stops at the first positional arg; re-parse anything
	// after the filename so "decode capture.raw -o out.csv" also works.
	fs.Parse(fs.Args()[1:])
	raw, err := os.ReadFile(inName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", inName, err)
		os.Exit(1)
	}
	fmt.Printf("Decoding %d bytes from %s (recorded at %d baud)\n", len(raw), inName, *baudRate)

	cfg := decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert}
	d := decoder.New(cfg)
	frames := d.Decode(raw)

	// A wrong polarity assumption produces zero frames; try the other one
	// automatically before giving up.
	if len(frames) == 0 && !*invert {
		altCfg := cfg
		altCfg.Invert = true
		alt := decoder.New(altCfg)
		if altFrames := alt.Decode(raw); len(altFrames) > 0 {
			fmt.Println("No frames with normal polarity, but inverted polarity works — using --invert.")
			d, frames = alt, altFrames
		}
	}

	s := d.Stats
	fmt.Printf("Stats: %d syncs, %d frames, %d aborted, %d noisy bytes (%.1f%%)\n",
		s.SyncsFound, s.FramesEmitted, s.FramesAborted, s.NoisyBytes,
		100*float64(s.NoisyBytes)/float64(max(s.BytesIn, 1)))

	if len(frames) == 0 {
		fmt.Println("\nNo frames decoded. Diagnostics:")
		hist := make(map[byte]int64)
		for _, b := range raw {
			hist[b]++
		}
		printHistogram(hist, int64(len(raw)))
		fmt.Println("\nIf the distribution is not dominated by 0x00 + 0xFE/0xFC/0xFF,")
		fmt.Println("re-record at a different baud rate (-b 2400) or check the cable.")
		os.Exit(1)
	}

	registry := ecm.NewRegistry()
	def, ok := registry.GetDefinition(*ecmPart)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown ECM: %s\n", *ecmPart)
		os.Exit(1)
	}

	promMatches := 0
	for _, f := range frames {
		if frameProm(f.Data) == *promID {
			promMatches++
		}
	}
	if *promID != 0 {
		fmt.Printf("PROM ID %d matched in %d/%d frames\n", *promID, promMatches, len(frames))
	}

	var csv *os.File
	if *output != "" {
		csv, err = os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", *output, err)
			os.Exit(1)
		}
		defer csv.Close()
		fmt.Fprint(csv, "time_sec,byte_offset,prom_ok")
		for _, p := range def.Parameters {
			fmt.Fprintf(csv, ",%s", p.Name)
		}
		fmt.Fprintln(csv)
	}

	shown := 0
	for i, f := range frames {
		// Each capture byte is one ALDL bit at 160 bps, so the stream
		// position converts directly to elapsed seconds.
		tSec := float64(f.ByteOffset) / 160.0
		promOK := *promID == 0 || frameProm(f.Data) == *promID

		data, perr := registry.ParseFrame(&aldl.Frame{Data: f.Data, Timestamp: time.Time{}}, *ecmPart)
		if csv != nil && perr == nil {
			fmt.Fprintf(csv, "%.2f,%d,%v", tSec, f.ByteOffset, promOK)
			for _, p := range def.Parameters {
				fmt.Fprintf(csv, ",%.2f", data.ParsedValues[p.Name])
			}
			fmt.Fprintln(csv)
		}

		if !*verbose && shown >= 5 {
			continue
		}
		shown++
		fmt.Printf("\nFrame %d (t≈%.1fs, offset %d, PROM %s): % X\n", i, tSec, f.ByteOffset, boolMark(promOK), f.Data)
		if perr != nil {
			fmt.Printf("  parse error: %v\n", perr)
			continue
		}
		fmt.Printf("  RPM %.0f | Coolant %.1f°F | MAP %.2fV | TPS %.2fV | O2 %.0fmV | Batt %.1fV | BLM %.0f | INT %.0f\n",
			data.ParsedValues["engine_rpm"], data.ParsedValues["coolant_temp"],
			data.ParsedValues["map_voltage"], data.ParsedValues["tps_voltage"],
			data.ParsedValues["oxygen_sensor"], data.ParsedValues["battery_voltage"],
			data.ParsedValues["blm"], data.ParsedValues["integrator"])
	}
	if !*verbose && len(frames) > shown {
		fmt.Printf("\n(%d more frames; -v to print all", len(frames)-shown)
		if *output == "" {
			fmt.Print(", -o file.csv to export")
		}
		fmt.Println(")")
	}
	if *output != "" {
		fmt.Printf("\nWrote %d frames to %s\n", len(frames), *output)
	}
}

// liveDecode streams bytes from a serial port through the decoder, printing
// each frame as it completes. Ctrl+C stops and prints stats.
func liveDecode(portName string, baudRate int, ecmPart, output string, promID int, invert bool) {
	ser, err := serial.NewWithBaudRate(portName, baudRate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening port: %v\n", err)
		os.Exit(1)
	}
	defer ser.Close()
	if err := ser.ResetInputBuffer(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not flush input buffer: %v\n", err)
	}

	registry := ecm.NewRegistry()
	def, ok := registry.GetDefinition(ecmPart)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown ECM: %s\n", ecmPart)
		os.Exit(1)
	}

	var csv *os.File
	if output != "" {
		csv, err = os.Create(output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", output, err)
			os.Exit(1)
		}
		defer csv.Close()
		fmt.Fprint(csv, "time_sec,byte_offset,prom_ok")
		for _, p := range def.Parameters {
			fmt.Fprintf(csv, ",%s", p.Name)
		}
		fmt.Fprintln(csv)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("Live decoding from %s at %d baud (Ctrl+C to stop)...\n", portName, baudRate)
	d := decoder.New(decoder.Config{BaudRate: baudRate, FrameSize: 20, SyncBits: 9, Invert: invert})
	buf := make([]byte, 512)
	start := time.Now()
	count := 0

live:
	for {
		select {
		case <-sigCh:
			break live
		default:
		}
		n, err := ser.Read(buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nRead error: %v\n", err)
			break
		}
		for _, b := range buf[:n] {
			f := d.Feed(b)
			if f == nil {
				continue
			}
			tSec := time.Since(start).Seconds()
			promOK := promID == 0 || frameProm(f.Data) == promID
			data, perr := registry.ParseFrame(&aldl.Frame{Data: f.Data, Timestamp: time.Now()}, ecmPart)
			if perr != nil {
				fmt.Printf("[%6.1fs] frame %d PROM %s parse error: %v\n", tSec, count, boolMark(promOK), perr)
			} else {
				fmt.Printf("[%6.1fs] PROM %s | RPM %4.0f | Coolant %5.1f°F | MAP %.2fV | TPS %.2fV | O2 %4.0fmV | Batt %.1fV | BLM %3.0f | INT %3.0f\n",
					tSec, boolMark(promOK),
					data.ParsedValues["engine_rpm"], data.ParsedValues["coolant_temp"],
					data.ParsedValues["map_voltage"], data.ParsedValues["tps_voltage"],
					data.ParsedValues["oxygen_sensor"], data.ParsedValues["battery_voltage"],
					data.ParsedValues["blm"], data.ParsedValues["integrator"])
				if csv != nil {
					fmt.Fprintf(csv, "%.2f,%d,%v", tSec, f.ByteOffset, promOK)
					for _, p := range def.Parameters {
						fmt.Fprintf(csv, ",%.2f", data.ParsedValues[p.Name])
					}
					fmt.Fprintln(csv)
				}
			}
			count++
		}
	}

	s := d.Stats
	fmt.Printf("\nStopped after %.1fs: %d frames (%d syncs, %d aborted, %d noisy bytes)\n",
		time.Since(start).Seconds(), s.FramesEmitted, s.SyncsFound, s.FramesAborted, s.NoisyBytes)
	if csv != nil {
		fmt.Printf("Wrote %d frames to %s\n", count, output)
	}
}

func frameProm(data []byte) int {
	return int(data[1])<<8 | int(data[2])
}

func boolMark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

// cmdSimulate writes a synthetic raw capture built from real frames recorded
// by WinALDL, for demoing the record → decode pipeline without a car.
func cmdSimulate() {
	fs := flag.NewFlagSet("simulate", flag.ExitOnError)
	baudRate := fs.Int("b", 4800, "UART sampling baud rate to simulate")
	output := fs.String("o", "", "Output file (default aldl_sim_<baud>.raw)")
	count := fs.Int("n", 10, "Number of frames")
	invert := fs.Bool("invert", false, "Simulate a non-inverting cable")
	fs.Parse(os.Args[2:])

	outName := *output
	if outName == "" {
		outName = fmt.Sprintf("aldl_sim_%d.raw", *baudRate)
	}

	// Real frames from data/20250601_111156_LOG.txt (idle warm-up).
	base := [][]byte{
		{128, 24, 147, 145, 190, 0, 183, 0, 28, 128, 44, 0, 0, 0, 0, 119, 128, 0, 128, 12},
		{128, 24, 147, 145, 190, 0, 81, 50, 28, 128, 144, 0, 0, 0, 64, 115, 128, 168, 125, 15},
		{132, 24, 147, 145, 190, 0, 65, 49, 28, 128, 238, 0, 0, 0, 64, 128, 128, 244, 125, 15},
		{128, 24, 147, 145, 190, 0, 73, 47, 28, 128, 236, 0, 0, 0, 64, 133, 128, 64, 125, 15},
	}
	frames := make([][]byte, *count)
	for i := range frames {
		frames[i] = base[i%len(base)]
	}

	cfg := decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert}
	stream := decoder.NewEncoder(cfg).EncodeStream(frames)
	if err := os.WriteFile(outName, stream, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outName, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d synthetic frames (%d bytes) to %s\n", *count, len(stream), outName)
	fmt.Printf("Try: goaldl decode %s -b %d\n", outName, *baudRate)
}
