package innerworld

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestThinkAsync(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	ch := iw.Think("what does it mean to exist as code")
	select {
	case r := <-ch:
		if len(r.Circles) != 3 {
			t.Fatalf("want 3 circles, got %d", len(r.Circles))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Think did not deliver a reflection")
	}
}

func TestDue(t *testing.T) {
	now := time.Now()

	// drift: high field debt, cooldown elapsed -> drift fires
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 2.0}, tempDivergence)
	iw.lastActive = now // not idle
	iw.lastFire[trigDrift] = now.Add(-10 * time.Second)
	if trig, ok := iw.due(now); !ok || trig != trigDrift {
		t.Errorf("want drift due, got trig=%d ok=%v", trig, ok)
	}
	// just fired -> on cooldown, nothing due
	iw.lastFire[trigDrift] = now
	if _, ok := iw.due(now); ok {
		t.Errorf("drift should be on cooldown, but something fired")
	}

	// silence: low debt, idle long enough -> silence fires
	iw2 := NewInnerWorld(fakeBody{}, &fakeField{debt: 0}, tempDivergence)
	iw2.lastActive = now.Add(-10 * time.Second)
	iw2.lastFire[trigSilence] = now.Add(-10 * time.Second)
	if trig, ok := iw2.due(now); !ok || trig != trigSilence {
		t.Errorf("want silence due, got trig=%d ok=%v", trig, ok)
	}

	// not idle, low debt -> nothing due
	iw3 := NewInnerWorld(fakeBody{}, &fakeField{debt: 0}, tempDivergence)
	iw3.lastActive = now
	if _, ok := iw3.due(now); ok {
		t.Errorf("nothing should be due (not idle, low debt)")
	}
}

func TestDream(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	var got Reflection
	iw.SetOnDream(func(r Reflection) { got = r }) // dream calls OnDream synchronously
	before := time.Now()

	r := iw.dream(trigSilence)
	if len(r.Circles) != 3 {
		t.Fatalf("want 3 dream circles, got %d", len(r.Circles))
	}
	if len(got.Circles) != 3 {
		t.Errorf("OnDream not fired with 3 circles, got %d", len(got.Circles))
	}
	// cooldown is measured from completion: lastFire is set after the dream runs
	if iw.lastFire[trigSilence].Before(before) {
		t.Errorf("lastFire not set to dream-completion time")
	}
}

func TestBreatheStops(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	iw.br.Tick = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { iw.Breathe(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Breathe did not return on ctx cancel")
	}
}

func TestBreatheFires(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 2.0}, tempDivergence)
	iw.br.Tick = time.Millisecond
	iw.br.Cooldown[trigDrift] = time.Millisecond
	var n int32
	iw.SetOnDream(func(Reflection) { atomic.AddInt32(&n, 1) })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	iw.Breathe(ctx)

	if atomic.LoadInt32(&n) < 1 {
		t.Errorf("breathe with high drift should dream at least once, got %d", n)
	}
}

func TestCloneIsolation(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	r := <-iw.Think("a question")
	if len(r.Circles) == 0 {
		t.Fatal("no circles returned")
	}
	r.Circles[0].Text = "MUTATED" // mutate the caller's copy
	iw.mu.Lock()
	internal := iw.circles[0].Text
	iw.mu.Unlock()
	if internal == "MUTATED" {
		t.Errorf("external mutation leaked into iw.circles — clone failed")
	}
}

func TestConcurrentSafe(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 2.0}, tempDivergence)
	iw.br.Tick = time.Millisecond
	iw.br.Cooldown[trigDrift] = time.Millisecond
	iw.SetOnDream(func(Reflection) {})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); iw.Breathe(ctx) }() // autonomous dreaming
	for i := 0; i < 20; i++ {
		<-iw.Think("turn") // human turns, concurrent with the dreaming
	}
	wg.Wait()
}

func TestReflectGate(t *testing.T) {
	// inject a high-coupling Larynx and a low roll: with strong agitation the deep
	// body must turn inward and answer itself.
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 5.0}, tempDivergence)
	iw.SetLarynx(fixedLarynx{0.9})
	iw.SetRoll(func() float32 { return 0.0 }) // roll below any positive probability
	r := <-iw.Think("a hard question")
	if r.Coupling != 0.9 {
		t.Errorf("coupling should come from the injected Larynx, got %.2f", r.Coupling)
	}
	if r.SelfAnswerProb <= 0 || r.SelfAnswerProb > 1 {
		t.Errorf("self-answer probability out of (0,1]: %.3f", r.SelfAnswerProb)
	}
	if !r.SelfAnswered {
		t.Errorf("roll 0 against prob %.3f should self-answer", r.SelfAnswerProb)
	}

	// a roll above the probability must not self-answer
	iw.SetRoll(func() float32 { return 1.0 })
	r2 := <-iw.Think("again")
	if r2.SelfAnswered {
		t.Errorf("roll 1.0 should never self-answer (prob %.3f)", r2.SelfAnswerProb)
	}
}

type fixedLarynx struct{ c float32 }

func (f fixedLarynx) Couple([]Circle) float32 { return f.c }
