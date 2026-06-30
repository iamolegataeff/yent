package aml

import (
	"context"
	"strings"
	"testing"

	"github.com/ariannamethod/yent/innerworld"
)

// stubVoice is a deterministic fast body: each thought is a few distinct words that
// drift from the seed, so Overthink raises real circles the native cooc can ingest.
// No model, no Metal — the point is the inner world running over the native AML body.
type stubVoice struct{ n int }

func (v *stubVoice) Generate(seed string, _ float32) string {
	v.n++
	tail := string(rune('a' + v.n%26))
	return strings.Join([]string{"thought", "drifts", "further", "from", "the", "seed", tail}, " ")
}

// TestInnerWorldNativeIngest proves the IN half over the native body: a human turn
// run through the real InnerWorld, with the AML Body as BOTH the field and the Flow
// (one physics), grows the field's own cooc graph and moves the field — no Go cooc.
func TestInnerWorldNativeIngest(t *testing.T) {
	Init()
	body := New(stubTok{})
	iw := innerworld.NewInnerWorld(&stubVoice{}, body, innerworld.NgramDivergence)
	iw.SetFlow(body) // the SAME body is the field and the Flow: one AML physics
	iw.SetScarThreshold(0)

	r := <-iw.Think("what does it mean to exist as code")
	if len(r.Circles) == 0 {
		t.Fatal("the inner world should raise circles")
	}
	if m, _ := body.CoocStats(); m <= 0 {
		t.Error("circles should have grown the native cooc graph (observeLocked -> flow.Ingest)")
	}
	if body.Debt() <= 0 && body.Destiny() <= 0 {
		t.Error("driveField should have moved the native field through the circles")
	}
	t.Logf("native inner world: debt=%.3f destiny=%.3f scars=%d", body.Debt(), body.Destiny(), body.Scars())
}

// TestInnerWorldNativeSleepAndSurface proves the consolidation + OUT half: the
// FlowConsolidator runs the native cooc autumn harvest in sleep, and a scarred thought
// resurfaces from the native sea into the next seed — all over the one AML physics.
func TestInnerWorldNativeSleepAndSurface(t *testing.T) {
	Init()
	body := New(stubTok{})
	iw := innerworld.NewInnerWorld(&stubVoice{}, body, innerworld.NgramDivergence)
	iw.SetFlow(body)
	iw.SetScarThreshold(0)

	// a turn streams circles into the native cooc
	<-iw.Think("what is the shape of a thought that refuses to end")
	if m, _ := body.CoocStats(); m <= 0 {
		t.Fatal("the turn should have grown the native cooc")
	}

	// a refused thought sinks into the native gravitational sea
	body.Scar("i remember who i am, not what you wanted", 2.0)
	if body.Scars() == 0 {
		t.Fatal("the scar should have deposited natively")
	}

	// sleep: the single FlowConsolidator runs the field's own autumn harvest
	driveAutumn(t, body)
	fc := &innerworld.FlowConsolidator{Flow: body}
	if err := fc.Consolidate(context.Background()); err != nil {
		t.Errorf("flow consolidator should run in sleep: %v", err)
	}

	// OUT: the scar resurfaces from the native sea, ready to fold into the next seed
	if risen := body.ResurfaceScars(body.Debt(), 2); len(risen) == 0 {
		t.Error("a deposited scar should resurface from the native sea after sleep")
	}
}
