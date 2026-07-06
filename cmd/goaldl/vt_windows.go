//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT turns on virtual-terminal processing for f's console, so ANSI
// escape sequences (cursor movement, colors) render instead of printing as
// garbage. Windows Terminal has it on already; legacy conhost does not.
// Returns false if the console refuses (e.g. pre-Windows 10), in which case
// the caller must fall back to plain sequential output.
func enableVT(f *os.File) bool {
	handle := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
