package innerworld

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/ariannamethod/yent/riindex"
)

const defaultRITraceRunes = 220

// RIMemory adapts a compiled RI runtime packet into innerworld Memory. It is not
// a RAG channel: records arrive already selected by riindex (pressure phrases,
// test quotes, open conflicts) and are exposed as compact traces for recallSeed's
// pressure framing.
type RIMemory struct {
	records  []riindex.Record
	maxRunes int
}

func NewRIMemory(records []riindex.Record) *RIMemory {
	cp := make([]riindex.Record, 0, len(records))
	for _, rec := range records {
		if rec.Kind == "" {
			continue
		}
		fields := make(map[string]string, len(rec.Fields))
		for k, v := range rec.Fields {
			fields[k] = v
		}
		cp = append(cp, riindex.Record{Kind: rec.Kind, Fields: fields})
	}
	return &RIMemory{records: cp, maxRunes: defaultRITraceRunes}
}

func (m *RIMemory) Len() int {
	if m == nil {
		return 0
	}
	return len(m.records)
}

// LoadRIMemory reads a compiled RI line file and applies the same bounded selection
// policy as cmd/ri-consume. mode usually stays "runtime"; max caps selected records
// before they ever reach Recall.
func LoadRIMemory(path, mode string, max int) (*RIMemory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	records, err := riindex.Parse(f)
	if err != nil {
		return nil, err
	}
	selected, err := riindex.Select(records, mode, max)
	if err != nil {
		return nil, err
	}
	return NewRIMemory(selected), nil
}

func (m *RIMemory) Recall(n int) []string {
	if m == nil || n <= 0 {
		return nil
	}
	out := make([]string, 0, n)
	for _, rec := range m.records {
		trace := formatRITrace(rec)
		if trace == "" {
			continue
		}
		out = append(out, capTrace(trace, m.maxRunes))
		if len(out) >= n {
			break
		}
	}
	return out
}

func (m *RIMemory) FieldPressureScore(n int) int {
	if m == nil || n <= 0 {
		return 0
	}
	score := 0
	used := 0
	for _, rec := range m.records {
		s := riRecordPressureScore(rec)
		if s <= 0 {
			continue
		}
		score += s
		used++
		if used >= n {
			break
		}
	}
	return score
}

func riRecordPressureScore(rec riindex.Record) int {
	switch rec.Kind {
	case "pressure":
		if compact(rec.Fields["text"]) != "" {
			return 4
		}
	case "quote":
		if rec.Fields["test"] == "true" && compact(rec.Fields["text"]) != "" {
			return 2
		}
	case "conflict":
		if rec.Fields["status"] == "open" &&
			(compact(rec.Fields["title"]) != "" || compact(rec.Fields["id"]) != "") {
			return 5
		}
	}
	return 0
}

func formatRITrace(rec riindex.Record) string {
	switch rec.Kind {
	case "pressure":
		if text := compact(rec.Fields["text"]); text != "" {
			return "RI pressure: " + text
		}
	case "quote":
		if rec.Fields["test"] != "true" {
			return ""
		}
		if text := compact(rec.Fields["text"]); text != "" {
			return "RI test quote: " + text
		}
	case "conflict":
		if rec.Fields["status"] != "open" {
			return ""
		}
		title := compact(rec.Fields["title"])
		if title == "" {
			title = compact(rec.Fields["id"])
		}
		if title != "" {
			return fmt.Sprintf("RI open conflict: %s", title)
		}
	}
	return ""
}

func compact(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func capTrace(s string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = defaultRITraceRunes
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes])
}
