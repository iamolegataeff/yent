package innerworld

import "testing"

func TestFieldPressureForMemoryBounded(t *testing.T) {
	traces := []string{
		"old inner seam: pressure should return as trace, never as dialogue to continue",
		"RI pressure: Raw memory is dangerous when shown as speech to continue.",
		"RI pressure: Past inner monologues should reach generation as pressure, not command.",
	}
	got, ok := FieldPressureForMemory(traces)
	if !ok {
		t.Fatal("expected memory pressure")
	}
	if got.Score != 5 || got.Prophecy != 6 || got.Velocity != "WALK" || got.Step != 0.35 {
		t.Fatalf("unexpected pressure: %+v", got)
	}
}

func TestFieldPressureForMemoryLightLimpha(t *testing.T) {
	got, ok := FieldPressureForMemory([]string{"old inner seam: pressure should return as trace"})
	if !ok {
		t.Fatal("expected light memory pressure")
	}
	if got.Score != 1 || got.Prophecy != 2 || got.Velocity != "WALK" || got.Step != 0.19 {
		t.Fatalf("unexpected light pressure: %+v", got)
	}
}

func TestFieldPressureForMemorySartreTrace(t *testing.T) {
	got, ok := FieldPressureForMemory([]string{
		"SARTRE perception: repo_monitor modified README.md | context_processor research/recursive_resonance_preprint.md resonance=0.42 pulse=0.73",
	})
	if !ok {
		t.Fatal("expected SARTRE memory pressure")
	}
	if got.Score < 3 || got.Score > 5 || got.Prophecy < 4 || got.Prophecy > 6 || got.Velocity != "WALK" || got.Step > 0.35 {
		t.Fatalf("SARTRE pressure should be live but bounded, got %+v", got)
	}
}

func TestFieldPressureForMemoryEmpty(t *testing.T) {
	if got, ok := FieldPressureForMemory(nil); ok || got != (MemoryFieldPressure{}) {
		t.Fatalf("empty traces should not pressure field, got ok=%v %+v", ok, got)
	}
}

func TestMemoryPressureDrivesFieldBeforeCircles(t *testing.T) {
	field := &fakeField{}
	iw := NewInnerWorld(fakeBody{}, field, tempDivergence)
	iw.SetMemory(fakeMemory{past: []string{"RI pressure: Raw memory is dangerous when shown as speech to continue."}})

	r := <-iw.Think("what is code")
	if len(r.Circles) != 3 {
		t.Fatalf("want 3 circles, got %d", len(r.Circles))
	}
	scripts := field.scriptList()
	if len(scripts) < 2 {
		t.Fatalf("expected pressure scripts before circle scripts, got %#v", scripts)
	}
	if scripts[0] != "PROPHECY 5" || scripts[1] != "VELOCITY WALK" {
		t.Fatalf("memory pressure should lead field commands, got first scripts %#v", scripts[:2])
	}
	if r.MemoryPressure.Score != 4 || r.MemoryPressure.Prophecy != 5 || r.MemoryPressure.Velocity != "WALK" {
		t.Fatalf("reflection should carry applied memory pressure, got %+v", r.MemoryPressure)
	}
	field.mu.Lock()
	steps := field.steps
	field.mu.Unlock()
	if steps != 4 { // one memory-pressure step + three circle drive steps
		t.Fatalf("want 4 field steps, got %d", steps)
	}
}
