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
	args := os.Args[1:]
	if len(args) > 0 {
		// A known command word runs that command; anything else (a flag, a
		// capture-file path, or nothing) is the dashboard. Files are *.raw, so
		// a positional that collides with a command name never happens in
		// practice — and `goaldl ./name` still resolves as a file.
		switch args[0] {
		case "record":
			cmdRecord(args[1:])
			return
		case "decode":
			cmdDecode(args[1:])
			return
		case "monitor":
			cmdMonitor(args[1:])
			return
		case "blm":
			cmdBLM(args[1:])
			return
		case "simulate":
			cmdSimulate(args[1:])
			return
		case "ports":
			cmdPorts()
			return
		case "ecms":
			cmdECMs()
			return
		case "help", "-h", "--help":
			printUsage()
			return
		}
	}
	// Default: the interactive dashboard is the face of goaldl.
	launchTUI(args)
}

// launchTUI starts the dashboard. With no arguments it auto-connects when
// exactly one USB serial port is present, so "goaldl" alone just works when a
// cable is plugged in.
func launchTUI(args []string) {
	if len(args) == 0 {
		if ports, _ := serial.AvailablePorts(); len(ports) == 1 {
			args = []string{"-p", ports[0]}
		}
	}
	cmdTUI(args)
}

func printUsage() {
	fmt.Println("goaldl - ALDL scanner and datalogger for GM ECMs")
	fmt.Println()
	fmt.Println("Interactive dashboard (default) — sensors · fuel-trim grids · flags · codes · raw:")
	fmt.Println("  goaldl -p /dev/cu.usbserial-10     live from the ECM")
	fmt.Println("  goaldl drive_4800.raw              replay a capture")
	fmt.Println("  goaldl                             auto-connect if one USB serial port is present")
	fmt.Println("  keys: 1-8 select tab · tab/←→ cycle · s save · c clear · r rec · d csv · space/± replay · q quit")
	fmt.Println()
	fmt.Println("Commands (scripting / headless):")
	fmt.Println("  record     Capture raw UART bytes to a file (no processing)")
	fmt.Println("  decode     Decode a capture file to frames + optional CSV")
	fmt.Println("  monitor    Streaming sensor or BLM table (non-interactive)")
	fmt.Println("  blm        Build a BLM fuel-trim table (rich/lean by RPM and load)")
	fmt.Println("  simulate   Generate a synthetic capture for testing decode")
	fmt.Println("  ports      List available USB serial ports")
	fmt.Println("  ecms       List supported ECM part numbers")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  goaldl ports")
	fmt.Println("  goaldl record -p /dev/cu.usbserial-10 -t 60 -o session.raw")
	fmt.Println("  goaldl decode session.raw -o frames.csv")
	fmt.Println("  goaldl blm session.raw -o correction.csv")
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
