package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"goaldl/pkg/decoder"
	"goaldl/pkg/ecm"
	"goaldl/pkg/stream"
)

// cmdMonitor renders a live sensor table (sensor / raw / translated value),
// driven by either a live ECM (-p) or a replayed capture file. Both paths feed
// the identical decode → parse → table pipeline.
//
//	goaldl monitor -p /dev/cu.usbserial-10 [-o capture.raw]   # live (optionally recording)
//	goaldl monitor drive_4800.raw [-speed 2]                  # replay a capture
func cmdMonitor() {
	fs := flag.NewFlagSet("monitor", flag.ExitOnError)
	portName := fs.String("p", "", "Live: serial port to read from (omit to replay a file)")
	baudRate := fs.Int("b", 4800, "UART sampling baud rate")
	ecmPart := fs.String("e", defaultECM, "ECM part number")
	promID := fs.Int("prom", 6291, "Expected PROM ID for the sync indicator (0 to disable)")
	invert := fs.Bool("invert", false, "Invert byte values (non-inverting cable)")
	record := fs.String("o", "", "Live only: also record raw bytes to this file")
	speed := fs.Float64("speed", 1.0, "Replay only: playback speed (1=real time, 0=as fast as possible)")
	fs.Parse(os.Args[2:])

	cfg := decoder.Config{BaudRate: *baudRate, FrameSize: 20, SyncBits: 9, Invert: *invert}
	registry := ecm.NewRegistry()
	if _, ok := registry.GetDefinition(*ecmPart); !ok {
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

	renderer := stream.NewRenderer(os.Stdout, isTTY(os.Stdout), registry, *ecmPart, *promID, title)
	var frames int
	err := provider.Run(ctx, func(ev stream.FrameEvent) {
		frames++
		renderer.Render(ev)
	})
	fmt.Printf("\nStopped after %d frames.\n", frames)
	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "Provider error: %v\n", err)
		os.Exit(1)
	}
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
