package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ariannamethod/yent/riindex"
)

func main() {
	in := flag.String("in", "ri/out/index.lines", "compiled RI line-protocol input")
	out := flag.String("out", "", "output path; stdout when empty")
	mode := flag.String("mode", "runtime", "selection mode: runtime, pressure, test-quotes, open-conflicts, all")
	max := flag.Int("max", 16, "maximum emitted records; <=0 means no cap")
	format := flag.String("format", "lines", "output format: lines or json")
	flag.Parse()

	f, err := os.Open(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ri-consume: open: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	records, err := riindex.Parse(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ri-consume: parse: %v\n", err)
		os.Exit(1)
	}
	selected, err := riindex.Select(records, *mode, *max)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ri-consume: select: %v\n", err)
		os.Exit(1)
	}
	data, err := riindex.Encode(riindex.NewPacket(*in, *mode, selected), *format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ri-consume: encode: %v\n", err)
		os.Exit(1)
	}
	if *out == "" {
		_, _ = os.Stdout.Write(data)
		return
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "ri-consume: write: %v\n", err)
		os.Exit(1)
	}
}
