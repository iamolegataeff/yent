package main

import (
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// HIGH-3 (Sol audit): the process-only boundary already reaches user speech through limpha recall, so D
// is PROMOTED to an honest first INDIRECT speech wire, and each persisted inner reflection carries a
// causal receipt of the Janus state that shaped it. janusReceipt() reads that state from the shared C
// kernel (driven here through the Go AMK wrapper, the same global G). Armed at a retrodiction date it
// records armed=true and a below-0.5 calendar signal; disarmed it records false.
func TestJanusReceiptCapturesArmedState(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Exec("SELF_NOW_DAYS 528") // janus_gap<0 -> retrodiction
	amk.Exec("JANUS_KEY 1")
	amk.Step(1.0)
	armed, alpha, gap := janusReceipt()
	if !armed {
		t.Fatal("receipt armed=false after JANUS_KEY 1, want true")
	}
	if gap >= 0 || alpha >= 0.5 {
		t.Fatalf("armed retrodiction receipt: gap=%.4f alpha=%.4f, want gap<0 and alpha<0.5", gap, alpha)
	}
	// Disarm: the receipt records unarmed even though the calendar signal is still computed.
	amk.Exec("JANUS_KEY 0")
	amk.Step(1.0)
	if a, _, _ := janusReceipt(); a {
		t.Fatal("receipt armed=true after JANUS_KEY 0, want false")
	}
}
