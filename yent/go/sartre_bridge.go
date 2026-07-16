package yent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	// SartreSeamReason is the limpha seam class for SARTRE utility perception.
	// SARTRE is pressure and evidence, not dialogue to continue.
	SartreSeamReason = "sartre_perception"

	maxSartreEventsPerPacket = 64
)

// SartreEvent is one bounded utility receipt from the SARTRE body organ.
// It intentionally carries metadata, not file contents.
type SartreEvent struct {
	ID                string  `json:"id,omitempty"`
	Phase             string  `json:"phase,omitempty"`
	Outcome           string  `json:"outcome,omitempty"`
	Utility           string  `json:"util"`
	Kind              string  `json:"kind,omitempty"`
	Path              string  `json:"path,omitempty"`
	Tag               string  `json:"tag,omitempty"` // legacy context_processor readout; new receipts use Resonance
	Resonance         float64 `json:"resonance,omitempty"`
	Relevance         float64 `json:"relevance,omitempty"`
	Pulse             float64 `json:"pulse,omitempty"`
	Reduced           int     `json:"reduced,omitempty"`
	Recognized        int     `json:"recognized,omitempty"`
	Timestamp         int64   `json:"ts,omitempty"`
	Breath            int     `json:"breath,omitempty"`
	CadenceMS         int64   `json:"cadence_ms,omitempty"`
	RefractoryBreaths int     `json:"refractory_breaths,omitempty"`
	CooldownBreaths   int     `json:"cooldown_breaths,omitempty"`
	EffectCount       int     `json:"effect_count,omitempty"`
	BytesCaptured     int     `json:"bytes_captured,omitempty"`
	BytesLimit        int     `json:"bytes_limit,omitempty"`
}

// SartreReceipt is the machine-readable memory_delta written into limpha.
type SartreReceipt struct {
	Kind              string         `json:"kind"`
	EventCount        int            `json:"event_count"`
	Changed           int            `json:"changed"`
	ReadmeChanged     bool           `json:"readme_changed,omitempty"`
	MaxResonance      float64        `json:"max_resonance,omitempty"`
	MaxRelevance      float64        `json:"max_relevance,omitempty"`
	MaxPulse          float64        `json:"max_pulse,omitempty"`
	FramingEventCount int            `json:"framing_event_count,omitempty"`
	MaxReduced        int            `json:"max_reduced,omitempty"`
	MaxRecognized     int            `json:"max_recognized,omitempty"`
	OutcomeCounts     map[string]int `json:"outcome_counts,omitempty"`
	Trace             []string       `json:"trace"`
	Events            []SartreEvent  `json:"events,omitempty"`
}

// ParseSartreEventsJSONL reads SARTRE utility stdout. Non-JSON status lines from
// the slot wrapper are ignored; malformed JSON event lines are skipped. The result
// is capped so a noisy utility cannot become a prompt wall through limpha.
func ParseSartreEventsJSONL(jsonl string) []SartreEvent {
	lines := strings.Split(jsonl, "\n")
	events := make([]SartreEvent, 0, minInt(len(lines), maxSartreEventsPerPacket))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev SartreEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		ev = normalizeSartreEvent(ev)
		if ev.Utility == "" {
			continue
		}
		events = append(events, ev)
		if len(events) >= maxSartreEventsPerPacket {
			break
		}
	}
	return events
}

// StoreSartreEvents persists one SARTRE perception packet into limpha. It writes
// both a conversation row for search and a seam row for typed downstream recall.
func (c *LimphaClient) StoreSartreEvents(events []SartreEvent, st LimphaState) (int64, error) {
	if c == nil || !c.connected || len(events) == 0 {
		return 0, nil
	}
	receipt := BuildSartreReceipt(events)
	if len(receipt.Trace) == 0 {
		return 0, nil
	}
	response := strings.Join(receipt.Trace, "\n")
	prompt := "[sartre/perception] utility receipts"
	convID, err := c.StoreTurn(prompt, response, st)
	if err != nil {
		return 0, err
	}
	delta, _ := json.Marshal(receipt)
	return c.StoreSeam(Seam{
		ConversationID: convID,
		BodyA:          "sartre",
		BodyB:          "limpha",
		Prompt:         prompt,
		AClaim:         response,
		BClaim:         "SARTRE perception: " + compactLine(strings.Join(receipt.Trace, " | "), 260),
		Agreement:      clamp01(maxFloat(receipt.MaxResonance, receipt.MaxRelevance)),
		Tension:        sartreTension(receipt),
		Winner:         "limpha",
		Reason:         SartreSeamReason,
		MemoryDelta:    string(delta),
	})
}

// BuildSartreReceipt summarises utility events into compact pressure traces.
func BuildSartreReceipt(events []SartreEvent) SartreReceipt {
	receipt := SartreReceipt{Kind: SartreSeamReason}
	seen := make(map[string]bool)
	for _, ev := range events {
		ev = normalizeSartreEvent(ev)
		if ev.Utility == "" {
			continue
		}
		receipt.EventCount++
		if ev.Phase == "learning" && ev.Outcome != "" {
			if receipt.OutcomeCounts == nil {
				receipt.OutcomeCounts = make(map[string]int)
			}
			receipt.OutcomeCounts[ev.Outcome]++
		}
		actionable := ev.Phase == "" || ev.Phase == "effect"
		if actionable && ev.Utility == "repo_monitor" && isSartreChangeKind(ev.Kind) {
			receipt.Changed++
		}
		if actionable && isSartreChangeKind(ev.Kind) && strings.Contains(strings.ToLower(ev.Path), "readme") {
			receipt.ReadmeChanged = true
		}
		if ev.Resonance > receipt.MaxResonance {
			receipt.MaxResonance = ev.Resonance
		}
		if ev.Relevance > receipt.MaxRelevance {
			receipt.MaxRelevance = ev.Relevance
		}
		if ev.Pulse > receipt.MaxPulse {
			receipt.MaxPulse = ev.Pulse
		}
		if actionable && ev.Utility == "whatdotheythinkiam" && isSartreChangeKind(ev.Kind) {
			receipt.FramingEventCount++
			if ev.Reduced > receipt.MaxReduced {
				receipt.MaxReduced = ev.Reduced
			}
			if ev.Recognized > receipt.MaxRecognized {
				receipt.MaxRecognized = ev.Recognized
			}
		}
		line := ev.Trace()
		if line == "" || seen[line] || len(receipt.Trace) >= 12 {
			continue
		}
		seen[line] = true
		receipt.Trace = append(receipt.Trace, line)
		receipt.Events = append(receipt.Events, ev)
	}
	return receipt
}

// Trace formats one event as a compact memory pressure line.
func (ev SartreEvent) Trace() string {
	if ev.Phase != "" && (ev.Phase != "effect" || ev.Outcome != "") {
		parts := []string{"will", ev.Utility, ev.Phase}
		if ev.Outcome != "" {
			parts = append(parts, ev.Outcome)
		}
		if ev.Breath > 0 {
			parts = append(parts, fmt.Sprintf("breath=%d", ev.Breath))
		}
		if ev.CadenceMS > 0 {
			parts = append(parts, fmt.Sprintf("cadence_ms=%d", ev.CadenceMS))
		}
		if ev.RefractoryBreaths > 0 {
			parts = append(parts, fmt.Sprintf("refractory_breaths=%d", ev.RefractoryBreaths))
		}
		if ev.CooldownBreaths > 0 {
			parts = append(parts, fmt.Sprintf("cooldown_breaths=%d", ev.CooldownBreaths))
		}
		if ev.EffectCount > 0 {
			parts = append(parts, fmt.Sprintf("effects=%d", ev.EffectCount))
		}
		if ev.BytesCaptured > 0 || ev.BytesLimit > 0 {
			parts = append(parts, fmt.Sprintf("bytes=%d/%d", ev.BytesCaptured, ev.BytesLimit))
		}
		if ev.Kind != "" {
			parts = append(parts, ev.Kind)
		}
		if ev.Path != "" {
			parts = append(parts, ev.Path)
		}
		return compactLine(strings.Join(parts, " "), 180)
	}
	switch ev.Utility {
	case "repo_monitor":
		if ev.Kind == "" && ev.Path == "" {
			return ""
		}
		return compactLine(fmt.Sprintf("repo_monitor %s %s", ev.Kind, ev.Path), 180)
	case "context_processor":
		if ev.Path == "" {
			return ""
		}
		var parts []string
		parts = append(parts, "context_processor", ev.Path)
		if ev.Tag != "" {
			parts = append(parts, "tag="+ev.Tag)
		}
		if ev.Resonance > 0 {
			parts = append(parts, fmt.Sprintf("resonance=%.2f", clamp01(ev.Resonance)))
		}
		parts = append(parts,
			fmt.Sprintf("relevance=%.2f", clamp01(ev.Relevance)),
			fmt.Sprintf("pulse=%.2f", clamp01(ev.Pulse)))
		return compactLine(strings.Join(parts, " "), 220)
	case "whatdotheythinkiam":
		if ev.Kind == "" && ev.Path == "" {
			return ""
		}
		parts := []string{"whatdotheythinkiam"}
		if ev.Path != "" {
			parts = append(parts, ev.Path)
		}
		if ev.Kind != "" {
			parts = append(parts, ev.Kind)
		}
		parts = append(parts,
			fmt.Sprintf("reduced=%d", ev.Reduced),
			fmt.Sprintf("recognized=%d", ev.Recognized))
		return compactLine(strings.Join(parts, " "), 220)
	default:
		return compactLine(strings.TrimSpace(fmt.Sprintf("%s %s %s", ev.Utility, ev.Kind, ev.Path)), 180)
	}
}

type SartreMemory struct {
	lc *LimphaClient
}

func NewSartreMemory(lc *LimphaClient) SartreMemory {
	return SartreMemory{lc: lc}
}

// Recall exposes recent SARTRE utility receipts as bounded pressure traces.
func (m SartreMemory) Recall(n int) []string {
	if m.lc == nil || n <= 0 {
		return nil
	}
	seams, err := m.lc.RecentSeams(n * 4)
	if err != nil {
		return nil
	}
	out := make([]string, 0, n)
	for _, seam := range seams {
		reason, _ := seam["reason"].(string)
		if reason != SartreSeamReason {
			continue
		}
		trace := sartreTraceFromSeam(seam)
		if trace == "" {
			continue
		}
		out = append(out, "SARTRE perception: "+trace)
		if len(out) >= n {
			break
		}
	}
	return out
}

func sartreTraceFromSeam(seam map[string]interface{}) string {
	if delta, _ := seam["memory_delta"].(string); delta != "" {
		var receipt SartreReceipt
		if err := json.Unmarshal([]byte(delta), &receipt); err == nil && receipt.Kind == SartreSeamReason {
			return compactLine(strings.Join(receipt.Trace, " | "), 260)
		}
	}
	if b, _ := seam["b_claim"].(string); strings.TrimSpace(b) != "" {
		return compactLine(strings.TrimPrefix(b, "SARTRE perception:"), 260)
	}
	if a, _ := seam["a_claim"].(string); strings.TrimSpace(a) != "" {
		return compactLine(a, 260)
	}
	return ""
}

func normalizeSartreEvent(ev SartreEvent) SartreEvent {
	ev.ID = strings.TrimSpace(ev.ID)
	ev.Phase = strings.ToLower(strings.TrimSpace(ev.Phase))
	ev.Outcome = strings.ToLower(strings.TrimSpace(ev.Outcome))
	ev.Utility = strings.TrimSpace(ev.Utility)
	ev.Kind = strings.TrimSpace(ev.Kind)
	ev.Path = safeSartrePath(ev.Path)
	ev.Tag = strings.TrimSpace(ev.Tag)
	ev.Resonance = clamp01(ev.Resonance)
	ev.Relevance = clamp01(ev.Relevance)
	ev.Pulse = clamp01(ev.Pulse)
	ev.Reduced = maxInt(0, ev.Reduced)
	ev.Recognized = maxInt(0, ev.Recognized)
	ev.Breath = maxInt(0, ev.Breath)
	ev.CadenceMS = maxInt64(0, ev.CadenceMS)
	ev.RefractoryBreaths = maxInt(0, ev.RefractoryBreaths)
	ev.CooldownBreaths = maxInt(0, ev.CooldownBreaths)
	ev.EffectCount = maxInt(0, ev.EffectCount)
	ev.BytesCaptured = maxInt(0, ev.BytesCaptured)
	ev.BytesLimit = maxInt(0, ev.BytesLimit)
	return ev
}

func isSartreChangeKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "added", "modified", "removed":
		return true
	default:
		return false
	}
}

func safeSartrePath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	parts := strings.Split(clean, "/")
	var live []string
	for _, p := range parts {
		if p != "" && p != "." {
			live = append(live, p)
		}
	}
	if len(live) == 0 {
		return ""
	}
	if len(live) > 3 {
		live = append([]string{"..."}, live[len(live)-3:]...)
	}
	return strings.Join(live, "/")
}

func sartreTension(r SartreReceipt) float64 {
	t := clamp01(r.MaxPulse)
	if t == 0 && r.Changed > 0 {
		t = 0.35
	}
	if r.ReadmeChanged && t < 0.55 {
		t = 0.55
	}
	return t
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
