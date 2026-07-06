package serial

import (
	"reflect"
	"testing"
)

func TestFilterUSBPorts(t *testing.T) {
	tests := []struct {
		name  string
		ports []string
		want  []string
	}{
		{
			name: "macOS chipsets matched, built-ins dropped",
			ports: []string{
				"/dev/cu.usbserial-10",        // PL2303 via DriverKit app / FTDI
				"/dev/cu.PL2303-USBtoUART210", // PL2303 via vendor driver
				"/dev/cu.wchusbserial14210",   // CH340 — regression: old prefix check could never match
				"/dev/cu.SLAB_USBtoUART",      // CP210x
				"/dev/cu.usbmodem14201",       // CDC-ACM
				"/dev/cu.Bluetooth-Incoming-Port",
				"/dev/cu.debug-console",
			},
			want: []string{
				"/dev/cu.usbserial-10",
				"/dev/cu.PL2303-USBtoUART210",
				"/dev/cu.wchusbserial14210",
				"/dev/cu.SLAB_USBtoUART",
				"/dev/cu.usbmodem14201",
			},
		},
		{
			name:  "Linux USB adapters matched, onboard UARTs dropped",
			ports: []string{"/dev/ttyUSB0", "/dev/ttyACM0", "/dev/ttyS0", "/dev/ttyAMA0"},
			want:  []string{"/dev/ttyUSB0", "/dev/ttyACM0"},
		},
		{
			name:  "Windows COM ports matched",
			ports: []string{"COM3", "COM12"},
			want:  []string{"COM3", "COM12"},
		},
		{
			name:  "BSD USB-serial matched, onboard dropped",
			ports: []string{"/dev/cuaU0", "/dev/cuau0"},
			want:  []string{"/dev/cuaU0"},
		},
		{
			name:  "no ports",
			ports: nil,
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterUSBPorts(tt.ports); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterUSBPorts(%v) = %v, want %v", tt.ports, got, tt.want)
			}
		})
	}
}
