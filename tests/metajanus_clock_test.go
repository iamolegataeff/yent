package tests

import (
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// MED-1 (Sol audit): the calendar epoch was built with local mktime, so it depended on the host
// timezone/DST — on an IDT+0300 host mktime gave 1727946000, not the UTC 1727956800, and hosts in
// different zones could disagree on the self-day near a day boundary. The fix pins the epoch to a fixed
// UTC instant, so the clock domain is host-independent. am_init sets it; NewAMK drives am_init.
func TestMetaJanusEpochIsUTCFixed(t *testing.T) {
	_ = yent.NewAMK() // runs am_init -> calendar_init
	const utcNoon = int64(1727956800) // 2024-10-03 12:00:00 UTC
	if got := yent.CalendarEpochSeconds(); got != utcNoon {
		t.Fatalf("calendar epoch = %d, want %d (2024-10-03 12:00 UTC, host-timezone-independent)", got, utcNoon)
	}
}
