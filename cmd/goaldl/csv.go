package main

import (
	"fmt"
	"os"

	"goaldl/pkg/ecm"
)

// frameCSV writes decoded frames as CSV: a fixed time/offset/prom_ok prefix
// followed by one column per ECM parameter. Shared by `decode` (batch) and
// `monitor --csv` (live/replay) so both emit the identical format.
type frameCSV struct {
	f    *os.File
	def  *ecm.Definition
	Rows int
}

func newFrameCSV(path string, def *ecm.Definition) (*frameCSV, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	fmt.Fprint(f, "time_sec,byte_offset,prom_ok")
	for _, p := range def.Parameters {
		fmt.Fprintf(f, ",%s", p.Name)
	}
	fmt.Fprintln(f)
	return &frameCSV{f: f, def: def}, nil
}

func (c *frameCSV) Write(tSec float64, byteOffset int64, promOK bool, parsed map[string]float64) {
	fmt.Fprintf(c.f, "%.2f,%d,%v", tSec, byteOffset, promOK)
	for _, p := range c.def.Parameters {
		fmt.Fprintf(c.f, ",%.2f", parsed[p.Name])
	}
	fmt.Fprintln(c.f)
	c.Rows++
}

func (c *frameCSV) Close() error { return c.f.Close() }
