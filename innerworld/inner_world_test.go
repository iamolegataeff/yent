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
	case circles := <-ch:
		if len(circles) != 3 {
			t.Fatalf("want 3 circles, got %d", len(circles))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Think did not deliver circles")
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
	var got []Circle
	iw.SetOnDream(func(c []Circle) { got = c }) // dream calls OnDream synchronously
	before := time.Now()

	circles := iw.dream(trigSilence)
	if len(circles) != 3 {
		t.Fatalf("want 3 dream circles, got %d", len(circles))
	}
	if len(got) != 3 {
		t.Errorf("OnDream not fired with 3 circles, got %d", len(got))
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
	iw.SetOnDream(func([]Circle) { atomic.AddInt32(&n, 1) })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	iw.Breathe(ctx)

	if atomic.LoadInt32(&n) < 1 {
		t.Errorf("breathe with high drift should dream at least once, got %d", n)
	}
}

func TestCloneIsolation(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	circles := <-iw.Think("a question")
	if len(circles) == 0 {
		t.Fatal("no circles returned")
	}
	circles[0].Text = "MUTATED" // mutate the caller's copy
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
	iw.SetOnDream(func([]Circle) {})

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
