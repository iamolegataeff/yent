package innerworld

import (
	"context"
	"strings"
	"testing"
)

func TestScarAccumulatesAndForgets(t *testing.T) {
	s := NewScarMemory(0.5) // fast decay for the test
	s.Scar("the refused thought", 2.0)
	s.Scar("the refused thought", 1.0) // recurring rejection accumulates gravity
	if n, total := s.Stats(); n != 1 || total != 3.0 {
		t.Errorf("recurring scar should accumulate: n=%d total=%.1f", n, total)
	}
	s.Scar("one-off", 0.5)

	// decay 0.5: "refused" 3->1.5 survives, "one-off" 0.5->0.25 < floor 0.4 forgotten.
	if forgotten := s.Consolidate(0.4); forgotten != 1 {
		t.Errorf("a faded scar should be forgotten, got %d", forgotten)
	}
	if n, _ := s.Stats(); n != 1 {
		t.Errorf("the recurring scar should survive, n=%d", n)
	}
}

func TestScarResurrect(t *testing.T) {
	s := NewScarMemory(0.985)
	s.Scar("deep wound", 5.0)
	s.Scar("light scratch", 1.0)

	risen := s.Resurrect(3.0, 5) // only gravity >= 3 surfaces
	if len(risen) != 1 || risen[0] != "deep wound" {
		t.Errorf("only scars above resonance surface, got %v", risen)
	}
	if len(s.Resurrect(0.5, 5)) != 2 {
		t.Error("low resonance should surface both scars")
	}
	if len(s.Resurrect(0, 5)) != 0 {
		t.Error("zero resonance surfaces nothing")
	}
}

func TestScarConsolidatorInSleep(t *testing.T) {
	s := NewScarMemory(0.5)
	s.Scar("fading", 0.5)
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	iw.AddConsolidator(&ScarConsolidator{Sea: s, PruneFloor: 0.4})

	iw.sleep(context.Background()) // 0.5*0.5=0.25 < 0.4 -> forgotten
	if n, _ := s.Stats(); n != 0 {
		t.Errorf("sleep should consolidate the scar sea, n=%d", n)
	}
}

func TestScarIntegration(t *testing.T) {
	// a high prophecy-debt thought is scarred; a later resonant debt resurfaces it.
	s := NewScarMemory(0.985)
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 5.0}, tempDivergence)
	iw.SetScar(s, 3.0) // debt 5 > threshold 3 -> scar

	<-iw.Think("a dissonant prompt")
	if n, _ := s.Stats(); n == 0 {
		t.Fatal("a high-debt thought should be scarred")
	}
	r := <-iw.Think("again")
	if len(r.Circles) == 0 {
		t.Fatal("no circles")
	}
	if !strings.Contains(r.Circles[0].Seed, "scar resurfaces") {
		t.Errorf("a resonant scar should resurface in the seed, got %q", r.Circles[0].Seed)
	}
}

func TestScarNilSafe(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 5.0}, tempDivergence)
	r := <-iw.Think("x")
	if len(r.Circles) == 0 {
		t.Fatal("no circles")
	}
	if strings.Contains(r.Circles[0].Seed, "scar resurfaces") {
		t.Errorf("no scar memory must not inject, got %q", r.Circles[0].Seed)
	}
}
