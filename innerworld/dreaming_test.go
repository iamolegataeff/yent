package innerworld

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeConsolidator records that a stage ran (atomic count) and in what order.
type fakeConsolidator struct {
	name    string
	counter *int32
	order   *[]string
	omu     *sync.Mutex
}

func (c *fakeConsolidator) Consolidate(_ context.Context) error {
	if c.counter != nil {
		atomic.AddInt32(c.counter, 1)
	}
	if c.order != nil {
		c.omu.Lock()
		*c.order = append(*c.order, c.name)
		c.omu.Unlock()
	}
	return nil
}

func (c *fakeConsolidator) Name() string { return c.name }

func waitForConsolidations(t *testing.T, counter *int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(counter) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d consolidation(s), got %d", want, atomic.LoadInt32(counter))
}

func TestSleepRunsConsolidatorsInOrder(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	var order []string
	var omu sync.Mutex
	for _, n := range []string{"cooc", "weights", "scar", "emotion"} {
		iw.AddConsolidator(&fakeConsolidator{name: n, order: &order, omu: &omu})
	}
	iw.sleep(context.Background())
	if iw.Asleep() {
		t.Error("organism should be awake after sleep returns")
	}
	if got := strings.Join(order, ","); got != "cooc,weights,scar,emotion" {
		t.Errorf("consolidators ran out of order: %q", got)
	}
}

func TestCriticalMass(t *testing.T) {
	// nil trigger never reaches critical mass (backward-compatible: no sleep).
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 5.0}, tempDivergence)
	if iw.criticalMass() {
		t.Error("nil sleep trigger must never reach critical mass")
	}
	// high debt over threshold -> critical mass.
	iw.SetSleepTrigger(func(f Field) bool { return f.Debt() >= 3.0 })
	if !iw.criticalMass() {
		t.Error("debt 5 >= 3 should reach critical mass")
	}
	// low debt -> stay awake.
	iw2 := NewInnerWorld(fakeBody{}, &fakeField{debt: 1.0}, tempDivergence)
	iw2.SetSleepTrigger(func(f Field) bool { return f.Debt() >= 3.0 })
	if iw2.criticalMass() {
		t.Error("debt 1 < 3 should stay awake")
	}
}

func TestBreatheSleepsOnCriticalMass(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 5.0}, tempDivergence)
	iw.br.Tick = time.Millisecond
	iw.SetSleepTrigger(func(f Field) bool { return f.Debt() >= 3.0 })
	var n int32
	iw.AddConsolidator(&fakeConsolidator{name: "cooc", counter: &n})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	iw.Breathe(ctx)

	if got := atomic.LoadInt32(&n); got != 1 {
		t.Errorf("Breathe should sleep once per continuous critical-mass episode, got %d", got)
	}
}

func TestBreatheSleepRearmsAfterCriticalFalls(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	iw.br.Tick = time.Millisecond
	iw.br.DriftDebt = 1000
	iw.br.Silence = time.Hour

	var critical atomic.Bool
	critical.Store(true)
	iw.SetSleepTrigger(func(Field) bool { return critical.Load() })
	var n int32
	iw.AddConsolidator(&fakeConsolidator{name: "flow", counter: &n})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		iw.Breathe(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	waitForConsolidations(t, &n, 1)
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt32(&n); got != 1 {
		t.Fatalf("critical mass stayed high; sleep must stay latched at 1, got %d", got)
	}

	critical.Store(false)
	time.Sleep(5 * time.Millisecond) // one below-threshold tick re-arms the next episode
	critical.Store(true)
	waitForConsolidations(t, &n, 2)
	if got := atomic.LoadInt32(&n); got != 2 {
		t.Errorf("after false->true critical mass should sleep exactly once more, got %d", got)
	}
}

func TestBreatheStaysAwakeBelowMass(t *testing.T) {
	// below threshold: no sleep, the dream path still runs (drift dreamer fires).
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 2.0}, tempDivergence)
	iw.br.Tick = time.Millisecond
	iw.br.Cooldown[trigDrift] = time.Millisecond
	iw.SetSleepTrigger(func(f Field) bool { return f.Debt() >= 100.0 }) // never reached
	var slept int32
	iw.AddConsolidator(&fakeConsolidator{name: "x", counter: &slept})
	var dreamt int32
	iw.SetOnDream(func(Reflection) { atomic.AddInt32(&dreamt, 1) })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	iw.Breathe(ctx)

	if atomic.LoadInt32(&slept) != 0 {
		t.Errorf("below critical mass must not consolidate, got %d", slept)
	}
	if atomic.LoadInt32(&dreamt) < 1 {
		t.Errorf("below critical mass the dream path should still fire, got %d", dreamt)
	}
}

type panicConsolidator struct{ name string }

func (panicConsolidator) Consolidate(context.Context) error { panic("boom") }
func (c panicConsolidator) Name() string                    { return c.name }

func TestSleepPanicContained(t *testing.T) {
	iw := NewInnerWorld(fakeBody{}, &fakeField{}, tempDivergence)
	var after int32
	iw.AddConsolidator(panicConsolidator{name: "boom"})
	iw.AddConsolidator(&fakeConsolidator{name: "after", counter: &after})

	iw.sleep(context.Background()) // a panicking stage must not propagate or wedge

	if iw.Asleep() {
		t.Error("asleep must be cleared even after a panicking stage")
	}
	if atomic.LoadInt32(&after) != 1 {
		t.Errorf("a later stage should still run after a panicking one, got %d", after)
	}
	// genMu must be free — a subsequent Think must not deadlock.
	select {
	case <-iw.Think("after sleep"):
	case <-time.After(time.Second):
		t.Fatal("genMu wedged after a panicking consolidator")
	}
}

func TestSleepConcurrentWithThink(t *testing.T) {
	// sleep runs in the breathe goroutine while human turns arrive; per-stage genMu
	// must keep it race-clean and let Think interleave between stages.
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 5.0}, tempDivergence)
	iw.br.Tick = time.Millisecond
	iw.SetSleepTrigger(func(f Field) bool { return f.Debt() >= 3.0 })
	iw.AddConsolidator(&fakeConsolidator{name: "a"})
	iw.AddConsolidator(&fakeConsolidator{name: "b"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); iw.Breathe(ctx) }()
	for i := 0; i < 20; i++ {
		<-iw.Think("turn")
	}
	wg.Wait()
}
