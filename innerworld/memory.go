package innerworld

// MergeMemory returns a fair, bounded Memory view over multiple sources. Each
// source is asked for up to n traces, then traces are interleaved source-by-source
// so a chatty limpha history cannot starve a smaller RI pressure packet.
func MergeMemory(sources ...Memory) Memory {
	var live []Memory
	for _, src := range sources {
		if src != nil {
			live = append(live, src)
		}
	}
	if len(live) == 0 {
		return nil
	}
	if len(live) == 1 {
		return live[0]
	}
	return mergedMemory{sources: live}
}

type mergedMemory struct {
	sources []Memory
}

func (m mergedMemory) Recall(n int) []string {
	out, _ := m.selectTraces(n)
	return out
}

func (m mergedMemory) FieldPressureScore(n int) int {
	_, selected := m.selectTraces(n)
	score := 0
	for _, src := range selected {
		if src.used <= 0 {
			continue
		}
		if scorer, ok := src.memory.(PressureMemory); ok {
			score += scorer.FieldPressureScore(src.used)
			continue
		}
		for _, trace := range src.traces[:src.used] {
			score += tracePressureScore(trace)
		}
	}
	return score
}

type selectedMemorySource struct {
	memory Memory
	traces []string
	used   int
}

func (m mergedMemory) selectTraces(n int) ([]string, []selectedMemorySource) {
	if n <= 0 || len(m.sources) == 0 {
		return nil, nil
	}
	sources := make([]selectedMemorySource, 0, len(m.sources))
	for _, src := range m.sources {
		if src == nil {
			continue
		}
		if got := src.Recall(n); len(got) > 0 {
			sources = append(sources, selectedMemorySource{memory: src, traces: got})
		}
	}
	if len(sources) == 0 {
		return nil, nil
	}
	out := make([]string, 0, n)
	for i := 0; len(out) < n; i++ {
		advanced := false
		for idx := range sources {
			if i >= len(sources[idx].traces) {
				continue
			}
			out = append(out, sources[idx].traces[i])
			sources[idx].used++
			advanced = true
			if len(out) >= n {
				break
			}
		}
		if !advanced {
			break
		}
	}
	return out, sources
}
