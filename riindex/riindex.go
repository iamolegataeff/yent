// Package riindex parses and filters the private RI line protocol.
//
// RI itself stays outside git under /ri. This package is the public-safe consumer
// surface: it only understands compact records emitted by cmd/ri-compile or
// cmd/ri-consume and lets runtime code select bounded pressure packets without
// knowing the private markdown corpus.
package riindex

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func NewPacket(input, mode string, records []Record) Packet {
	return Packet{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Input:       input,
		Mode:        mode,
		Records:     records,
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
	if !validMode(mode) {
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
	var selected []Record
	for _, rec := range records {
		if want(rec, mode) {
			selected = append(selected, rec)
			if max > 0 && len(selected) >= max {
				return selected, nil
			}
		}
	}
	return selected, nil
}

func validMode(mode string) bool {
	switch mode {
	case "runtime", "pressure", "test-quotes", "open-conflicts", "all":
		return true
	default:
		return false
	}
}

func want(rec Record, mode string) bool {
	switch mode {
	case "all":
		return rec.Kind != "ri" && rec.Kind != "packet"
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
