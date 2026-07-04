package main

import (
	"fmt"
	"os"

	"goaldl/pkg/ecm"
	"goaldl/pkg/serial"
)

const (
	defaultECM = "1227747"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "record":
		cmdRecord()
	case "decode":
		cmdDecode()
	case "simulate":
		cmdSimulate()
	case "monitor":
		cmdMonitor()
	case "tui":
		cmdTUI()
	case "blm":
		cmdBLM()
	case "ports":
		cmdPorts()
	case "ecms":
		cmdECMs()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("goaldl - ALDL protocol scanner and datalogger for GM ECMs")
	fmt.Println()
	fmt.Println("Usage: goaldl <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  record             Capture raw UART bytes to a file (no processing)")
	fmt.Println("  tui                Interactive dashboard: navigate sensors / BLM / raw, live or replay")
	fmt.Println("  monitor            Live sensor table from a port (-p) or a replayed capture file")
	fmt.Println("  decode             Decode a capture file to frames + optional CSV")
	fmt.Println("  blm                Build a BLM fuel-trim table (rich/lean by RPM and load) from a capture")
	fmt.Println("  simulate           Generate a synthetic capture for testing decode")
	fmt.Println("  ports              List available USB serial ports")
	fmt.Println("  ecms               List supported ECM part numbers")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  goaldl ports")
	fmt.Println("  goaldl record -p /dev/cu.usbserial-10 -t 60 -o session.raw")
	fmt.Println("  goaldl tui pkg/decoder/testdata/drive_4800.raw              # interactive dashboard")
	fmt.Println("  goaldl monitor pkg/decoder/testdata/drive_4800.raw          # replay as a live table")
	fmt.Println("  goaldl monitor -p /dev/cu.usbserial-10 -o session.raw -csv live.csv  # live table + record + log")
	fmt.Println("  goaldl decode session.raw -o frames.csv                     # batch decode + export")
	fmt.Println("  goaldl blm session.raw -o correction.csv                    # BLM fuel-trim table (rich/lean map)")
	fmt.Println("  goaldl monitor -p /dev/cu.usbserial-10 -blm -o session.raw  # live BLM grid + record")
	fmt.Println("  goaldl simulate -n 10 && goaldl decode aldl_sim_4800.raw")
}

// cmdPorts lists available serial ports.
func cmdPorts() {
	ports, err := serial.AvailablePorts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing ports: %v\n", err)
		os.Exit(1)
	}
	if len(ports) == 0 {
		fmt.Println("No serial ports found")
		return
	}
	fmt.Println("Available serial ports:")
	for _, port := range ports {
		fmt.Printf("  %s\n", port)
	}
}

// cmdECMs lists supported ECMs.
func cmdECMs() {
	registry := ecm.NewRegistry()
	for _, part := range registry.ListSupportedECMs() {
		def, _ := registry.GetDefinition(part)
		fmt.Printf("  %s - %s\n", part, def.Description)
	}
}
