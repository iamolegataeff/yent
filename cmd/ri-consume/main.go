package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Record struct {
	Kind   string            `json:"kind"`
	Fields map[string]string `json:"fields"`
}

type Packet struct {
	GeneratedAt string   `json:"generated_at"`
	Input       string   `json:"input"`
	Mode        string   `json:"mode"`
	Records     []Record `json:"records"`
}

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

	records, err := Parse(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ri-consume: parse: %v\n", err)
		os.Exit(1)
	}
	selected, err := Select(records, *mode, *max)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ri-consume: select: %v\n", err)
		os.Exit(1)
	}
	packet := Packet{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Input:       *in,
		Mode:        *mode,
		Records:     selected,
	}
	data, err := Encode(packet, *format)
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

func Parse(r io.Reader) ([]Record, error) {
	var records []Record
	sc := bufio.NewScanner(r)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rec, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		records = append(records, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func parseLine(line string) (Record, error) {
	parts := strings.Split(line, "\t")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return Record{}, errors.New("missing record kind")
	}
	rec := Record{Kind: parts[0], Fields: map[string]string{}}
	for _, part := range parts[1:] {
		k, v, ok := strings.Cut(part, "=")
		if !ok || k == "" {
			return Record{}, fmt.Errorf("bad field %q", part)
		}
		rec.Fields[k] = unescapeField(v)
	}
	return rec, nil
}

func Select(records []Record, mode string, max int) ([]Record, error) {
	var selected []Record
	for _, rec := range records {
		if want(rec, mode) {
			selected = append(selected, rec)
			if max > 0 && len(selected) >= max {
				return selected, nil
			}
		}
	}
	if mode != "runtime" && mode != "pressure" && mode != "test-quotes" && mode != "open-conflicts" && mode != "all" {
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
	return selected, nil
}

func want(rec Record, mode string) bool {
	switch mode {
	case "all":
		return rec.Kind != "ri"
	case "pressure":
		return rec.Kind == "pressure"
	case "test-quotes":
		return rec.Kind == "quote" && parseBool(rec.Fields["test"])
	case "open-conflicts":
		return rec.Kind == "conflict" && rec.Fields["status"] == "open"
	case "runtime":
		return rec.Kind == "pressure" ||
			(rec.Kind == "quote" && parseBool(rec.Fields["test"])) ||
			(rec.Kind == "conflict" && rec.Fields["status"] == "open")
	default:
		return false
	}
}

func parseBool(s string) bool {
	v, _ := strconv.ParseBool(s)
	return v
}

func Encode(packet Packet, format string) ([]byte, error) {
	switch format {
	case "json":
		data, err := json.MarshalIndent(packet, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	case "lines":
		return []byte(encodeLines(packet)), nil
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

func encodeLines(packet Packet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "packet\tmode=%s\tinput=%s\tcount=%d\n", escapeField(packet.Mode), escapeField(packet.Input), len(packet.Records))
	for _, rec := range packet.Records {
		fmt.Fprintf(&b, "%s", rec.Kind)
		for _, key := range sortedKeys(rec.Fields) {
			fmt.Fprintf(&b, "\t%s=%s", key, escapeField(rec.Fields[key]))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func escapeField(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func unescapeField(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		if esc {
			switch r {
			case 't':
				b.WriteByte('\t')
			case 'n':
				b.WriteByte('\n')
			default:
				b.WriteRune(r)
			}
			esc = false
			continue
		}
		if r == '\\' {
			esc = true
			continue
		}
		b.WriteRune(r)
	}
	if esc {
		b.WriteByte('\\')
	}
	return b.String()
}
