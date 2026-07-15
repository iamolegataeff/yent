package innerworld

import "testing"

// HIGH-1 (Sol audit): JANUS_KEY must gate the CONSUMER, not just the writer. While unarmed, D-2 must
// read a neutral alpha (0.5 -> harvest 3/2 bit-for-bit) even if temporal_alpha is frozen off-center
// (after JANUS_KEY 0) or driven by a legacy TEMPORAL_ALPHA/REMEMBER_FUTURE directive. Armed, the poled
// alpha drives the lean. Reproduces Sol's key-off and legacy-directive scenarios at the consumer boundary.
func TestMetaJanusKeyGatesConsumer(t *testing.T) {
	// Unarmed with a frozen/poled alpha must still read neutral -> harvest (3,2).
	unarmed := fakeArmedFlow{alpha: 0.003, armed: false}
	if a := metajanusTemporalAlpha(unarmed); a != 0.5 {
		t.Fatalf("unarmed flow (poled alpha=%.4f) read as %.4f, want 0.5 (D-2 must be neutral while unarmed)", unarmed.alpha, a)
	}
	if b, s := metajanusHarvestLean(metajanusTemporalAlpha(unarmed)); b != 3 || s != 2 {
		t.Fatalf("unarmed harvest = (%d,%d), want (3,2) bit-for-bit", b, s)
	}
	// A legacy directive drives alpha to 1.0, but the key was never armed -> still neutral.
	legacy := fakeArmedFlow{alpha: 1.0, armed: false}
	if b, s := metajanusHarvestLean(metajanusTemporalAlpha(legacy)); b != 3 || s != 2 {
		t.Fatalf("legacy-directive unarmed harvest = (%d,%d), want (3,2) (D-2 must not wake without JANUS_KEY)", b, s)
	}
	// Armed: the poled alpha drives the lean.
	armed := fakeArmedFlow{alpha: 0.003, armed: true}
	if b, s := metajanusHarvestLean(metajanusTemporalAlpha(armed)); b != 1 || s != 4 {
		t.Fatalf("armed harvest = (%d,%d), want (1,4) (retrodiction pole drives the lean)", b, s)
	}
}

// fakeArmedFlow satisfies Flow via the embedded *goFlow (methods promoted, never called here) and adds
// the two Janus signals D-2 reads: the value and whether the key is armed.
type fakeArmedFlow struct {
	*goFlow
	alpha float32
	armed bool
}

func (f fakeArmedFlow) JanusTemporalAlpha() float32 { return f.alpha }
func (f fakeArmedFlow) JanusKeyArmed() bool         { return f.armed }
