package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"goaldl/pkg/aldl"
	"goaldl/pkg/ecm"
	"goaldl/pkg/logging"
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
	case "ports":
		cmdPorts()
	case "ecms":
		cmdECMs()
	case "test":
		cmdTest()
	case "convert":
		cmdConvert()
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
	fmt.Println("  decode             Decode a capture file, or live from a port with -p")
	fmt.Println("  monitor            Live sensor table from a port (-p) or a replayed capture file")
	fmt.Println("  simulate           Generate a synthetic capture for testing decode")
	fmt.Println("  ports              List available USB serial ports")
	fmt.Println("  ecms               List supported ECM part numbers")
	fmt.Println("  test               Parse a hex capture file and print sensors")
	fmt.Println("  convert            Convert a hex capture file to CSV")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  goaldl ports")
	fmt.Println("  goaldl record -p /dev/cu.usbserial-10 -t 60")
	fmt.Println("  goaldl decode aldl_capture_4800.raw -o frames.csv")
	fmt.Println("  goaldl decode -p /dev/cu.usbserial-10 -o live.csv   # real-time")
	fmt.Println("  goaldl monitor pkg/decoder/testdata/drive_4800.raw  # replay as a live table")
	fmt.Println("  goaldl monitor -p /dev/cu.usbserial-10 -o session.raw   # live table + record")
	fmt.Println("  goaldl simulate -n 10 && goaldl decode aldl_sim_4800.raw")
	fmt.Println("  goaldl test data/varied_sensors.hex")
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

// cmdTest parses a hex capture file (one frame of hex per line) and prints
// decoded sensors for the first few frames. Useful for checking sensor
// formulas against a known capture without hardware.
func cmdTest() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: goaldl test <hex_file>\n")
		os.Exit(1)
	}
	hexFile := os.Args[2]

	file, err := os.Open(hexFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	registry := ecm.NewRegistry()
	scanner := bufio.NewScanner(file)
	frameCount := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		data, err := hex.DecodeString(strings.ReplaceAll(line, " ", ""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing hex: %v\n", err)
			continue
		}
		ecmData, err := registry.ParseFrame(&aldl.Frame{Data: data, Timestamp: time.Now()}, defaultECM)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing frame: %v\n", err)
			continue
		}
		frameCount++
		if frameCount <= 5 {
			displayData(ecmData)
		}
	}
	fmt.Printf("\nProcessed %d frames from %s\n", frameCount, hexFile)
}

// cmdConvert converts a hex capture file to CSV, interpolating timestamps.
func cmdConvert() {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	outputPath := fs.String("o", "output.csv", "Output CSV file path")
	interval := fs.Int("i", 100, "Timestamp interval in milliseconds")
	fs.Parse(os.Args[2:])

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: goaldl convert <hex_file> -o output.csv -i 100\n")
		os.Exit(1)
	}
	hexFile := fs.Arg(0)

	file, err := os.Open(hexFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	logger, err := logging.New(*outputPath, logging.FormatCSV)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	registry := ecm.NewRegistry()
	scanner := bufio.NewScanner(file)
	frameCount := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		data, err := hex.DecodeString(strings.ReplaceAll(line, " ", ""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing hex: %v\n", err)
			continue
		}
		frame := &aldl.Frame{
			Data:      data,
			Timestamp: time.Now().Add(time.Duration(frameCount*(*interval)) * time.Millisecond),
		}
		ecmData, err := registry.ParseFrame(frame, defaultECM)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing frame: %v\n", err)
			continue
		}
		if err := logger.LogData(ecmData); err != nil {
			fmt.Fprintf(os.Stderr, "Error logging data: %v\n", err)
			continue
		}
		frameCount++
	}
	fmt.Printf("Converted %d frames to %s\n", frameCount, *outputPath)
}

// displayData prints a decoded frame's sensors in human-readable form.
func displayData(data *ecm.Data) {
	fmt.Println("---")
	fmt.Printf("ECM: %s\n", data.EcmDefinition.PartNumber)
	fmt.Printf("Raw: %s\n", hex.EncodeToString(data.RawData))
	fmt.Println("Sensors:")
	fmt.Printf("  Coolant Temp: %.1f°F\n", data.ParsedValues["coolant_temp"])
	fmt.Printf("  Vehicle Speed: %.0f MPH\n", data.ParsedValues["vehicle_speed"])
	fmt.Printf("  Engine RPM: %.0f RPM\n", data.ParsedValues["engine_rpm"])
	fmt.Printf("  MAP: %.4f V\n", data.ParsedValues["map_voltage"])
	fmt.Printf("  TPS: %.4f V\n", data.ParsedValues["tps_voltage"])
	fmt.Printf("  O2 Sensor: %.2f mV\n", data.ParsedValues["oxygen_sensor"])
	fmt.Printf("  Battery: %.1f V\n", data.ParsedValues["battery_voltage"])
}
