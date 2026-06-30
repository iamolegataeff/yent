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
	if n <= 0 || len(m.sources) == 0 {
		return nil
	}
	lists := make([][]string, 0, len(m.sources))
	for _, src := range m.sources {
		if src == nil {
			continue
		}
		if got := src.Recall(n); len(got) > 0 {
			lists = append(lists, got)
		}
	}
	if len(lists) == 0 {
		return nil
	}
	out := make([]string, 0, n)
	for i := 0; len(out) < n; i++ {
		advanced := false
		for _, list := range lists {
			if i >= len(list) {
				continue
			}
			out = append(out, list[i])
			advanced = true
			if len(out) >= n {
				break
			}
		}
		if !advanced {
			break
		}
	}
	return out
}
