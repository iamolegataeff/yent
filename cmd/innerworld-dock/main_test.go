package main

import (
	"testing"
	"time"
)

func TestDurationEnv(t *testing.T) {
	const k = "YENT_TEST_DURATION"
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", 0},          // unset/blank -> default
		{"300", 300 * time.Second},
		{"0", 0},         // non-positive -> default
		{"-5", 0},        // negative -> default
		{"NaN", 0},       // NaN must be rejected, not flow into time.Duration
		{"Inf", 0},       // +Inf rejected
		{"+Inf", 0},      // +Inf rejected
		{"-Inf", 0},      // -Inf is <= 0, rejected
		{"1e400", 0},     // parses to +Inf -> rejected
		{"abc", 0},       // unparseable -> default
		{"1.5", time.Duration(1.5 * float64(time.Second))},
	}
	for _, c := range cases {
		t.Setenv(k, c.raw)
		if got := durationEnv(k); got != c.want {
			t.Errorf("durationEnv(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}
