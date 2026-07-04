package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"goaldl/pkg/aldl"
	"goaldl/pkg/blm"
	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/stream"
)

// cmdMonitor renders a live sensor table (sensor / raw / translated value),
// driven by either a live ECM (-p) or a replayed capture file. Both paths feed
// the identical decode → parse → table pipeline.
//
//	goaldl monitor -p /dev/cu.usbserial-10 [-o capture.raw] [-csv frames.csv]   # live
//	goaldl monitor drive_4800.raw [-speed 2] [-csv frames.csv]                  # replay
func cmdMonitor() {
	fs := flag.NewFlagSet("monitor", flag.ExitOnError)
	portName := fs.String("p", "", "Live: serial port to read from (omit to replay a file)")
	baudRate := fs.Int("b", 4800, "UART sampling baud rate")
	ecmPart := fs.String("e", defaultECM, "ECM part number")
	promID := fs.Int("prom", 6291, "Expected PROM ID for the sync indicator (0 to disable)")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	record := fs.String("o", "", "Live only: also record raw bytes to this file")
	csvOut := fs.String("csv", "", "Also export decoded frames to this CSV file (sensor view)")
	blmView := fs.Bool("blm", false, "Show a live BLM fuel-trim grid instead of the sensor table")
	minSamples := fs.Int("min", blm.DefaultMinSamples, "BLM view: samples before a cell is trusted (shown solid vs dim)")
	speed := fs.Float64("speed", 1.0, "Replay only: playback speed (1=real time, 0=as fast as possible)")
	fs.Parse(os.Args[2:])

	cfg := decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert}
	registry := ecm.NewRegistry()
	def, ok := registry.GetDefinition(*ecmPart)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown ECM: %s\n", *ecmPart)
		os.Exit(1)
	}

	var provider stream.Provider
	var title string

	if *portName != "" {
		var sink *os.File
		if *record != "" {
			f, err := os.Create(*record)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", *record, err)
				os.Exit(1)
			}
			defer f.Close()
			sink = f
		}
		provider = &stream.SerialProvider{Port: *portName, Baud: *baudRate, Config: cfg, Sink: sink}
		title = "goaldl monitor — live " + *portName
		if *record != "" {
			title += " (recording " + *record + ")"
		}
	} else {
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Usage: goaldl monitor -p <port>   |   goaldl monitor <capture.raw>")
			os.Exit(1)
		}
		inName := fs.Arg(0)
		fs.Parse(fs.Args()[1:]) // allow flags after the filename
		data, err := os.ReadFile(inName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", inName, err)
			os.Exit(1)
		}
		provider = &stream.ReplayProvider{Data: data, Config: cfg, Speed: *speed}
		title = "goaldl monitor — replay " + inName
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *blmView {
		runBLMView(ctx, provider, "goaldl BLM — "+strings.TrimPrefix(title, "goaldl monitor — "), *minSamples)
		return
	}

	// Sensor-table view with optional CSV export. CSV is created after the
	// replay branch re-parses trailing flags, so `monitor <file> -csv out.csv`
	// is honored.
	var csv *frameCSV
	if *csvOut != "" {
		c, err := newFrameCSV(*csvOut, def)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", *csvOut, err)
			os.Exit(1)
		}
		defer c.Close()
		csv = c
	}

	renderer := stream.NewRenderer(os.Stdout, isTTY(os.Stdout), registry, *ecmPart, *promID, title)
	var frames int
	err := provider.Run(ctx, func(ev stream.FrameEvent) {
		frames++
		renderer.Render(ev)
		if csv != nil {
			promOK := *promID == 0 || frameProm(ev.Frame.Data) == *promID
			if data, perr := registry.ParseFrame(&aldl.Frame{Data: ev.Frame.Data}, *ecmPart); perr == nil {
				csv.Write(ev.Elapsed.Seconds(), ev.Frame.ByteOffset, promOK, data.ParsedValues)
			}
		}
	})
	fmt.Printf("\nStopped after %d frames.\n", frames)
	if csv != nil {
		fmt.Printf("Wrote %d frames to %s\n", csv.Rows, *csvOut)
	}
	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "Provider error: %v\n", err)
		os.Exit(1)
	}
}

// runBLMView streams frames through a live BLM grid, then prints the final
// Wide Average and Correction tables when the stream ends. Cells below
// minSamples are held at 1.0 in the correction table.
func runBLMView(ctx context.Context, provider stream.Provider, title string, minSamples int) {
	view := stream.NewBLMView(os.Stdout, isTTY(os.Stdout), title, minSamples)
	err := provider.Run(ctx, view.Render)
	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "\nProvider error: %v\n", err)
		os.Exit(1)
	}
	g := view.Grid
	fmt.Printf("\n\nFinal — %d closed-loop samples, %d cells reached %d+ (trusted)\n\n",
		g.TotalSamples(), g.PopulatedCells(minSamples), minSamples)
	fmt.Print(g.RenderFloat("Wide Average BLM (target 128; >128 lean, <128 rich)", g.Average(), 1))
	fmt.Println()
	fmt.Printf("Correction factor = avg/128 (cells with <%d samples held at 1.000)\n", minSamples)
	fmt.Print(g.RenderFloat("", g.CorrectionAtLeast(minSamples), 3))
}

// isTTY reports whether f is an interactive terminal, so the renderer knows
// whether it may redraw in place with cursor movement.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
