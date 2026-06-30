package innerworld

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// CoocGraph is the inner co-occurrence memory: which words the organism's own
// thoughts keep firing together. Circles seed it — the inner world grows richer
// than the dataset (haze-style emergence) — and the dream sleep consolidates it the
// arianna way: reinforce the strong edges, decay and prune the weak. It is
// word-level and pure Go; the token-level AML cooc graph (`am_cooc_consolidate`) is
// a later wiring when limpha/RI join. Concurrency-safe.
type CoocGraph struct {
	mu     sync.Mutex
	edges  map[string]map[string]float32 // src word -> dst word -> weight
	window int                           // co-occurrence window (words)
}

// NewCoocGraph builds an empty graph with the given co-occurrence window (>=1).
func NewCoocGraph(window int) *CoocGraph {
	if window < 1 {
		window = 2
	}
	return &CoocGraph{edges: map[string]map[string]float32{}, window: window}
}

func coocWords(text string) []string {
	return strings.Fields(strings.ToLower(text))
}

// Observe folds a thought's word co-occurrences into the graph: every ordered pair
// within the window gains weight. This is the circles->field direction — the
// organism's thoughts enrich its own memory.
func (g *CoocGraph) Observe(text string) {
	words := coocWords(text)
	if len(words) < 2 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := 0; i < len(words); i++ {
		for j := i + 1; j < len(words) && j <= i+g.window; j++ {
			src, dst := words[i], words[j]
			if src == dst {
				continue
			}
			row := g.edges[src]
			if row == nil {
				row = map[string]float32{}
				g.edges[src] = row
			}
			row[dst]++
		}
	}
}

// Consolidate is the arianna seasonal harvest (the logic of ariannamethod.c:7037 on
// word edges): edges at or above the median weight are reinforced by (1+reinforce),
// below are decayed by (1-reinforce), and edges that fall under pruneFloor are
// dropped — forgetting the long tail. Returns the number of edges pruned.
func (g *CoocGraph) Consolidate(reinforce, pruneFloor float32) int {
	if reinforce < 0 {
		reinforce = 0
	}
	if reinforce > 0.9 {
		reinforce = 0.9
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	// Gather all weights for the median split.
	var weights []float32
	for _, row := range g.edges {
		for _, w := range row {
			weights = append(weights, w)
		}
	}
	if len(weights) == 0 {
		return 0
	}
	sort.Slice(weights, func(a, b int) bool { return weights[a] < weights[b] })
	median := weights[len(weights)/2]

	const cntCap = 1e6
	pruned := 0
	for src, row := range g.edges {
		for dst, w := range row {
			if w >= median {
				w *= 1 + reinforce
			} else {
				w *= 1 - reinforce
			}
			if w > cntCap {
				w = cntCap // clamp BOTH branches: an already-over-cap edge cannot survive above the cap
			}
			if w < pruneFloor {
				delete(row, dst)
				pruned++
			} else {
				row[dst] = w
			}
		}
		if len(row) == 0 {
			delete(g.edges, src)
		}
	}
	return pruned
}

// Bias returns up to n strongest destination words for a seed word — the graph's
// pull on the next thought (the field->circles direction). Empty if the seed word
// is unknown.
func (g *CoocGraph) Bias(seedWord string, n int) []string {
	if n <= 0 {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	row := g.edges[strings.ToLower(seedWord)]
	if len(row) == 0 {
		return nil
	}
	type pair struct {
		w   string
		cnt float32
	}
	pairs := make([]pair, 0, len(row))
	for w, c := range row {
		pairs = append(pairs, pair{w, c})
	}
	sort.Slice(pairs, func(a, b int) bool { return pairs[a].cnt > pairs[b].cnt })
	if n > len(pairs) {
		n = len(pairs)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = pairs[i].w
	}
	return out
}

// Stats reports the live edge count and total weight (telemetry).
func (g *CoocGraph) Stats() (edges int, total float32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, row := range g.edges {
		for _, w := range row {
			edges++
			total += w
		}
	}
	return edges, total
}

// CoocConsolidator is the Б1 consolidation stage: the seasonal harvest of the inner
// co-occurrence graph, run during sleep. Reinforce/floor are the harvest strength
// (in production scaled by the field's autumn energy).
type CoocConsolidator struct {
	Graph      *CoocGraph
	Reinforce  float32
	PruneFloor float32
}

func (c *CoocConsolidator) Consolidate(_ context.Context) error {
	if c.Graph == nil {
		return nil
	}
	c.Graph.Consolidate(c.Reinforce, c.PruneFloor)
	return nil
}

func (c *CoocConsolidator) Name() string { return "cooc" }

// SetCooc wires the inner co-occurrence graph. With it, circles seed the graph
// (circles->field) and the graph pulls the next thought (field->circles) — the
// bidirectional loop. Set before Think/Breathe start.
func (iw *InnerWorld) SetCooc(g *CoocGraph) {
	iw.genMu.Lock()
	iw.cooc = g
	iw.genMu.Unlock()
}

// coocBias prepends the graph's pull on the prompt's last word — the words the
// organism's own thoughts keep associating with it — so the cooc field shapes the
// next circle (the field->circles half of the loop). No graph / unknown word =
// unchanged. NO-SEED-FROM-PROMPT holds: the result is still transformed by innerSeed
// inside Overthink.
func (iw *InnerWorld) coocBias(prompt string) string {
	var pull []string
	switch {
	case iw.flow != nil:
		pull = iw.flow.BiasWords(prompt, 3) // native cooc: the field's own token graph
	case iw.cooc != nil:
		if words := coocWords(prompt); len(words) > 0 {
			pull = iw.cooc.Bias(words[len(words)-1], 3)
		}
	}
	if len(pull) == 0 {
		return prompt
	}
	return "(inner pull: " + strings.Join(pull, " ") + ") " + prompt
}

// observeLocked folds the just-raised circles back into the cooc graph — the
// circles->field half of the loop, so the inner world grows richer than the dataset.
// Caller holds genMu (the graph has its own lock; this only keeps the order with the
// single voice consistent).
func (iw *InnerWorld) observeLocked(circles []Circle) {
	if iw.flow != nil {
		for _, c := range circles {
			iw.flow.Ingest(c.Text) // native cooc: am_ingest_tokens grows the field's graph
		}
		return
	}
	if iw.cooc == nil {
		return
	}
	for _, c := range circles {
		iw.cooc.Observe(c.Text)
	}
}
