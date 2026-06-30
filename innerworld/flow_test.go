package innerworld

import "testing"

// compile-time: the Go fallback body satisfies the Flow interface.
var _ Flow = (*goFlow)(nil)

func TestGoFlowIngestAndScar(t *testing.T) {
	cooc := NewCoocGraph(2)
	scar := NewScarMemory(0.5)
	f := NewGoFlow(&fakeField{}, cooc, scar, 0.3, 0.8, 0.4)

	f.Ingest("light meets shadow") // stream into the body
	if e, _ := cooc.Stats(); e == 0 {
		t.Error("Ingest should grow the cooc graph")
	}
	f.Scar("a refused thought", 2.0)
	if n, _ := scar.Stats(); n != 1 {
		t.Error("Scar should sink into the sea")
	}

	// consolidation through the flow drives the underlying organs.
	if f.ConsolidateCooc() < 0 {
		t.Error("ConsolidateCooc should run")
	}
	f.ConsolidateScar() // 2.0*0.5=1.0 >= floor 0.4, survives; exercise the path
	if n, _ := scar.Stats(); n != 1 {
		t.Errorf("a heavy scar should survive one consolidation, n=%d", n)
	}
}

func TestGoFlowAutumnEnergy(t *testing.T) {
	f := NewGoFlow(&fakeField{debt: 3.0}, nil, nil, 0, 0, 0)
	if e := f.AutumnEnergy(); e <= 0 || e >= 1 {
		t.Errorf("autumn energy should saturate in (0,1) for debt 3, got %.3f", e)
	}
	if f0 := NewGoFlow(&fakeField{debt: 0}, nil, nil, 0, 0, 0); f0.AutumnEnergy() != 0 {
		t.Error("zero debt -> zero autumn energy")
	}
}

func TestGoFlowNilOrgansSafe(t *testing.T) {
	f := NewGoFlow(&fakeField{}, nil, nil, 0.3, 0.8, 0.4)
	f.Ingest("x y z")       // no cooc -> no-op
	f.Scar("rejected", 1.0) // no scar -> no-op
	if f.ConsolidateCooc() != 0 || f.ConsolidateScar() != 0 {
		t.Error("nil organs should consolidate nothing")
	}
	f.ApplyPressure(nil) // honest no-op, must not panic
}

func TestGoFlowBiasAndResurface(t *testing.T) {
	cooc := NewCoocGraph(2)
	scar := NewScarMemory(0.5)
	f := NewGoFlow(&fakeField{}, cooc, scar, 0.3, 0.8, 0.4)

	f.Ingest("light meets shadow")
	f.Ingest("light meets shadow") // reinforce the edges
	if got := f.BiasWords("a poem about light", 3); len(got) == 0 {
		t.Error("BiasWords should pull the cooc neighbours of the seed's last word")
	}
	f.Scar("a refused thought", 2.0)
	if got := f.ResurfaceScars(1.0, 2); len(got) == 0 {
		t.Error("ResurfaceScars should surface a scar above the resonance level")
	}

	// nil organs stay safe
	g := NewGoFlow(&fakeField{}, nil, nil, 0, 0, 0)
	if g.BiasWords("x", 3) != nil || g.ResurfaceScars(1, 2) != nil {
		t.Error("nil organs -> nil bias/resurface")
	}
}

func TestGoFlowIsField(t *testing.T) {
	// the body IS the field: the embedded Field methods are reachable through Flow.
	field := &fakeField{debt: 1.5}
	var fl Flow = NewGoFlow(field, nil, nil, 0, 0, 0)
	if fl.Debt() != 1.5 {
		t.Errorf("flow should expose the field's debt, got %.2f", fl.Debt())
	}
	if err := fl.Exec("PROPHECY 7"); err != nil {
		t.Errorf("flow should pass AML commands to the field, got %v", err)
	}
}
