// Package aldl holds the shared ALDL frame type. Decoding lives in
// pkg/decoder; this package is intentionally minimal.
package aldl

import "time"

// Frame represents a single decoded ALDL data frame.
type Frame struct {
	Data      []byte
	Timestamp time.Time
}
