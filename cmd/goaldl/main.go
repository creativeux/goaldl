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
		switch args[0] {
		case "cli":
			runCLI(args[1:])
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

// runCLI dispatches the scripting/headless commands, kept under a "cli"
// namespace so the dashboard can own the bare `goaldl` invocation.
func runCLI(args []string) {
	if len(args) == 0 {
		printCLIUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "record":
		cmdRecord(args[1:])
	case "decode":
		cmdDecode(args[1:])
	case "monitor":
		cmdMonitor(args[1:])
	case "blm":
		cmdBLM(args[1:])
	case "simulate":
		cmdSimulate(args[1:])
	case "ports":
		cmdPorts()
	case "ecms":
		cmdECMs()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: goaldl cli %s\n", args[0])
		printCLIUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("goaldl - ALDL scanner and datalogger for GM ECMs")
	fmt.Println()
	fmt.Println("Interactive dashboard (default) — tab between sensors / BLM grid / raw:")
	fmt.Println("  goaldl -p /dev/cu.usbserial-10     live from the ECM")
	fmt.Println("  goaldl drive_4800.raw              replay a capture")
	fmt.Println("  goaldl                             auto-connect if one USB serial port is present")
	fmt.Println("  keys: 1-3 / tab switch views · q quit")
	fmt.Println()
	fmt.Println("Scripting / headless commands live under 'cli':")
	fmt.Println("  goaldl cli ports                          list serial ports")
	fmt.Println("  goaldl cli record -p <port> -t 60 -o session.raw")
	fmt.Println("  goaldl cli decode session.raw -o frames.csv")
	fmt.Println("  goaldl cli blm session.raw -o correction.csv")
	fmt.Println("  goaldl cli                                list all cli commands")
	fmt.Println()
	fmt.Println("  goaldl help                               this help")
}

func printCLIUsage() {
	fmt.Println("goaldl cli - scripting and headless commands")
	fmt.Println()
	fmt.Println("  record     Capture raw UART bytes to a file (no processing)")
	fmt.Println("  decode     Decode a capture file to frames + optional CSV")
	fmt.Println("  monitor    Live/replay sensor or BLM table (streaming, non-interactive)")
	fmt.Println("  blm        Build a BLM fuel-trim table (rich/lean by RPM and load)")
	fmt.Println("  simulate   Generate a synthetic capture for testing decode")
	fmt.Println("  ports      List available USB serial ports")
	fmt.Println("  ecms       List supported ECM part numbers")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  goaldl cli record -p /dev/cu.usbserial-10 -t 60 -o session.raw")
	fmt.Println("  goaldl cli decode session.raw -o frames.csv")
	fmt.Println("  goaldl cli blm session.raw -o correction.csv")
	fmt.Println("  goaldl cli simulate -n 10 && goaldl cli decode aldl_sim_4800.raw")
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
