package innerworld

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ariannamethod/yent/riindex"
)

func TestMergeMemoryInterleavesSources(t *testing.T) {
	mem := MergeMemory(
		fakeMemory{past: []string{"limpha newest", "limpha older", "limpha oldest"}},
		fakeMemory{past: []string{"ri pressure", "ri quote"}},
	)
	got := mem.Recall(4)
	want := []string{"limpha newest", "ri pressure", "limpha older", "ri quote"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("bad merge order: got %#v want %#v", got, want)
	}
}

func TestRIMemoryFormatsBoundedRuntimePacket(t *testing.T) {
	mem := NewRIMemory([]riindex.Record{
		{Kind: "pressure", Fields: map[string]string{"text": "  RI   must stay structure. "}},
		{Kind: "quote", Fields: map[string]string{"test": "true", "text": "Don't become me."}},
		{Kind: "quote", Fields: map[string]string{"test": "false", "text": "non-test quote"}},
		{Kind: "conflict", Fields: map[string]string{"status": "open", "title": "KK/Dario vs Prompt Stuffing"}},
		{Kind: "conflict", Fields: map[string]string{"status": "resolved", "title": "resolved conflict"}},
		{Kind: "node", Fields: map[string]string{"title": "not runtime"}},
	})
	got := mem.Recall(4)
	if len(got) != 3 {
		t.Fatalf("want 3 runtime traces, got %d: %#v", len(got), got)
	}
	if got[0] != "RI pressure: RI must stay structure." {
		t.Fatalf("bad pressure trace: %q", got[0])
	}
	if got[1] != "RI test quote: Don't become me." {
		t.Fatalf("bad quote trace: %q", got[1])
	}
	if got[2] != "RI open conflict: KK/Dario vs Prompt Stuffing" {
		t.Fatalf("bad conflict trace: %q", got[2])
	}
}

func TestRIMemoryProvidesTypedFieldPressure(t *testing.T) {
	mem := NewRIMemory([]riindex.Record{
		{Kind: "pressure", Fields: map[string]string{"text": "Plain curated pressure."}},
		{Kind: "quote", Fields: map[string]string{"test": "true", "text": "Don't become me."}},
		{Kind: "conflict", Fields: map[string]string{"status": "open", "title": "KK/Dario vs Prompt Stuffing"}},
	})
	got, ok := FieldPressureFromMemory(mem, 1)
	if !ok {
		t.Fatal("expected RI field pressure")
	}
	if got.Score != 4 || got.Prophecy != 5 || got.Step != 0.31 {
		t.Fatalf("RI typed pressure should not depend on trace wording, got %+v", got)
	}
	got, ok = FieldPressureFromMemory(mem, 3)
	if !ok || got.Score != 5 || got.Prophecy != 6 || got.Step != 0.35 {
		t.Fatalf("RI pressure should cap when multiple records are recalled, got ok=%v %+v", ok, got)
	}
}

func TestMergedMemoryPressureFollowsInterleavedRecallWindow(t *testing.T) {
	ri := NewRIMemory([]riindex.Record{
		{Kind: "conflict", Fields: map[string]string{"status": "open", "title": "Heavy RI conflict"}},
	})
	mem := MergeMemory(
		fakeMemory{past: []string{"limpha neutral trace"}},
		ri,
	)

	got, ok := FieldPressureFromMemory(mem, 1)
	if !ok {
		t.Fatal("expected pressure from the selected limpha trace")
	}
	if got.Score != 1 || got.Prophecy != 2 || got.Step != 0.19 {
		t.Fatalf("RI must not pressure field before it enters recall window, got %+v", got)
	}

	got, ok = FieldPressureFromMemory(mem, 2)
	if !ok || got.Score != 5 || got.Prophecy != 6 || got.Step != 0.35 {
		t.Fatalf("RI pressure should apply once selected by interleave, got ok=%v %+v", ok, got)
	}
}

func TestLoadRIMemoryUsesRuntimeSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.lines")
	if err := os.WriteFile(path, []byte(`ri	generated_at=x	root=ri
pressure	text=pressure survives
quote	test=true	text=test quote survives
quote	test=false	text=non-test quote must not leak
conflict	status=open	title=open conflict survives
conflict	status=resolved	title=resolved conflict must not leak
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mem, err := LoadRIMemory(path, "runtime", 0)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(mem.Recall(10), "\n")
	for _, want := range []string{"pressure survives", "test quote survives", "open conflict survives"} {
		if !strings.Contains(text, want) {
			t.Fatalf("runtime packet missing %q in:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"non-test quote", "resolved conflict"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("runtime packet leaked %q in:\n%s", forbidden, text)
		}
	}
}
