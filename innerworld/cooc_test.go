package innerworld

import (
	"context"
	"strings"
	"testing"
)

func TestCoocObserveGrows(t *testing.T) {
	g := NewCoocGraph(2)
	if e, _ := g.Stats(); e != 0 {
		t.Fatalf("empty graph should have 0 edges, got %d", e)
	}
	g.Observe("the void breathes slow")
	if e, total := g.Stats(); e == 0 || total == 0 {
		t.Errorf("observe should grow the graph, got edges=%d total=%.1f", e, total)
	}
}

func TestCoocConsolidateReinforcesStrongPrunesWeak(t *testing.T) {
	g := NewCoocGraph(2)
	for i := 0; i < 4; i++ {
		g.Observe("light meets shadow") // strong pairs, weight 4
	}
	g.Observe("odd lone token") // weak pairs, weight 1
	before, _ := g.Stats()

	// median split: weak (1×0.7=0.7) falls under floor 0.8 and is pruned; strong
	// (4×1.3=5.2) survives reinforced — the arianna seasonal harvest.
	pruned := g.Consolidate(0.3, 0.8)
	after, _ := g.Stats()
	if pruned == 0 || after >= before {
		t.Errorf("weak edges should be pruned: before=%d after=%d pruned=%d", before, after, pruned)
	}
	if len(g.Bias("light", 1)) == 0 {
		t.Error("a strong edge from 'light' should survive consolidation")
	}
}

func TestCoocBias(t *testing.T) {
	g := NewCoocGraph(2)
	for i := 0; i < 5; i++ {
		g.Observe("code dreams")
	}
	g.Observe("code rusts")
	pull := g.Bias("code", 2)
	if len(pull) == 0 || pull[0] != "dreams" {
		t.Errorf("strongest pull from 'code' should be 'dreams', got %v", pull)
	}
	if len(g.Bias("unknownword", 3)) != 0 {
		t.Error("unknown seed word should yield no pull")
	}
}

// multiWordBody returns a multi-word thought, so circle text yields cooc word pairs
// (fakeBody returns a single bracketed token, which Observe skips).
type multiWordBody struct{}

func (multiWordBody) Generate(string, float32) string { return "the tide turns again and again" }

func TestCoocBidirectional(t *testing.T) {
	g := NewCoocGraph(3)
	iw := NewInnerWorld(multiWordBody{}, &fakeField{}, tempDivergence)
	iw.SetCooc(g)

	// circles->field: thinking grows the graph.
	<-iw.Think("the sea remembers")
	if e, _ := g.Stats(); e == 0 {
		t.Fatal("circles should seed the cooc graph (circles->field)")
	}

	// field->circles: a strong primed edge must pull the next seed.
	for i := 0; i < 5; i++ {
		g.Observe("code dreams")
	}
	r := <-iw.Think("code")
	if len(r.Circles) == 0 {
		t.Fatal("no circles")
	}
	if !strings.Contains(r.Circles[0].Seed, "inner pull") {
		t.Errorf("seed should carry the cooc pull (field->circles), got %q", r.Circles[0].Seed)
	}
}

func TestCoocConsolidatorInSleep(t *testing.T) {
	g := NewCoocGraph(2)
	for i := 0; i < 3; i++ {
		g.Observe("alpha beta gamma")
	}
	g.Observe("rare odd token")
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	iw.AddConsolidator(&CoocConsolidator{Graph: g, Reinforce: 0.3, PruneFloor: 0.8})

	before, _ := g.Stats()
	iw.sleep(context.Background())
	after, _ := g.Stats()
	if after >= before {
		t.Errorf("sleep should consolidate the cooc graph: before=%d after=%d", before, after)
	}
}

func TestCoocNilSafe(t *testing.T) {
	// no cooc graph -> coocBias is a no-op, observe is a no-op, Think unaffected.
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	r := <-iw.Think("plain")
	if len(r.Circles) == 0 {
		t.Fatal("no circles")
	}
	if strings.Contains(r.Circles[0].Seed, "inner pull") {
		t.Errorf("no cooc graph must not inject pull, got %q", r.Circles[0].Seed)
	}
}
