//go:build !windows

package main

import "os"

// enableVT is a no-op outside Windows: every Unix terminal that reports as a
// character device interprets ANSI escapes.
func enableVT(*os.File) bool { return true }
