package yent

import (
	"path/filepath"
	"testing"
)

// fakeBody is a deterministic Body for router tests (no model, no doe).
type fakeBody struct {
	name       string
	answer     string
	confidence float64
	verdict    *Verdict
	calls      int
}

func (b *fakeBody) Name() string { return b.name }
func (b *fakeBody) Generate(prompt, ctx string) (BodyResult, error) {
	b.calls++
	return BodyResult{Answer: b.answer, Confidence: b.confidence, Verdict: b.verdict}, nil
}

func newRouterLimpha(t *testing.T) *LimphaClient {
	t.Helper()
	c, err := NewLimphaClientAt(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("limpha: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestRouterFastBodyAnswersAlone(t *testing.T) {
	lc := newRouterLimpha(t)
	fast := &fakeBody{name: "nemo12", answer: "quick answer", confidence: 0.9}
	deep := &fakeBody{name: "small24", answer: "deep answer", confidence: 1.0}
	r := NewRouter(fast, deep, lc)
	out, err := r.Route("hi there", LimphaState{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Escalated || out.Body != "nemo12" || out.Answer != "quick answer" {
		t.Errorf("confident fast should answer alone: %+v", out)
	}
	if deep.calls != 0 {
		t.Errorf("deep body must not run, got %d calls", deep.calls)
	}
	s, _ := lc.Stats()
	if s["total_conversations"].(int64) != 1 || s["total_seams"].(int64) != 0 {
		t.Errorf("want 1 conv / 0 seams, got %v / %v", s["total_conversations"], s["total_seams"])
	}
}

func TestRouterEscalatesOnLowConfidence(t *testing.T) {
	lc := newRouterLimpha(t)
	fast := &fakeBody{name: "nemo12", answer: "unsure", confidence: 0.2}
	deep := &fakeBody{name: "small24", answer: "considered answer",
		verdict: &Verdict{Agreement: 0.4, Tension: 0.7, Winner: "small24"}}
	r := NewRouter(fast, deep, lc)
	out, err := r.Route("what is qualia", LimphaState{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Escalated || out.Reason != "low_confidence" || out.Body != "small24" {
		t.Errorf("low-confidence should escalate to deep: %+v", out)
	}
	if deep.calls != 1 {
		t.Errorf("deep must run once, got %d", deep.calls)
	}
	if out.SeamID == 0 {
		t.Fatal("escalated turn must write a seam")
	}
	rs, _ := lc.RecentSeams(1)
	if len(rs) != 1 {
		t.Fatalf("want 1 seam, got %d", len(rs))
	}
	m := rs[0]
	if m["body_a"] != "nemo12" || m["b_claim"] != "considered answer" ||
		!approx(m["agreement"].(float64), 0.4) || !approx(m["tension"].(float64), 0.7) ||
		m["winner"] != "small24" || m["reason"] != "low_confidence" {
		t.Errorf("seam internal-dialogue/metrics wrong: %v", m)
	}
	// the stored conversation is the deep (winning) answer
	rec, _ := lc.Recent(1, false)
	if len(rec) != 1 || rec[0]["response"] != "considered answer" {
		t.Errorf("winning answer should be stored, got %v", rec)
	}
}

func TestRouterEscalatesOnComplexity(t *testing.T) {
	lc := newRouterLimpha(t)
	fast := &fakeBody{name: "nemo12", answer: "shallow", confidence: 0.95}
	deep := &fakeBody{name: "small24", answer: "architecture explained",
		verdict: &Verdict{Agreement: 0.5, Tension: 0.5, Winner: "small24"}}
	r := NewRouter(fast, deep, lc)
	out, _ := r.Route("explain the architecture of the system", LimphaState{})
	if !out.Escalated || out.Reason != "complexity" {
		t.Errorf("complex prompt should escalate even when fast is confident: %+v", out)
	}
	if deep.calls != 1 {
		t.Errorf("deep must run once on complexity, got %d", deep.calls)
	}
}

func TestRouterDeepConcedesFastWins(t *testing.T) {
	lc := newRouterLimpha(t)
	fast := &fakeBody{name: "nemo12", answer: "fast was right", confidence: 0.2}
	deep := &fakeBody{name: "small24", answer: "deep deferred",
		verdict: &Verdict{Agreement: 0.9, Tension: 0.1, Winner: "nemo12"}}
	r := NewRouter(fast, deep, lc)
	out, _ := r.Route("ambiguous", LimphaState{})
	if out.Body != "nemo12" || out.Answer != "fast was right" {
		t.Errorf("when deep concedes, the fast answer wins: %+v", out)
	}
	if !out.Escalated || out.SeamID == 0 {
		t.Error("still a dual-pass turn — seam must be recorded")
	}
	// stored conversation is the conceded-to fast answer
	rec, _ := lc.Recent(1, false)
	if len(rec) != 1 || rec[0]["response"] != "fast was right" {
		t.Errorf("conceded fast answer should be stored, got %v", rec)
	}
}

func TestRouterNilLimphaNoPanic(t *testing.T) {
	fast := &fakeBody{name: "nemo12", answer: "ok", confidence: 0.2}
	deep := &fakeBody{name: "small24", answer: "deep", verdict: &Verdict{Winner: "small24"}}
	r := NewRouter(fast, deep, nil) // memory disabled
	out, err := r.Route("explain the algorithm", LimphaState{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Escalated || out.SeamID != 0 {
		t.Errorf("nil limpha: escalate but no seam id, got %+v", out)
	}
}
