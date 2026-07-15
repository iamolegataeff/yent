package tests

import (
	"math"
	"strconv"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// Stage C — observation without intervention (Fable audit plan). This probe projects the whole
// trajectory of the four MetaJanus fields across ~2 years from the origin via the SELF_NOW_DAYS
// test-door, which moves the observation clock only — never the origin, never generation. It is the
// lens Fable asked for: the janus_gap sawtooth, the personal_dissonance trajectory, and the yahrzeit
// pulse at the anniversary windows, so keying (stage D) builds on watched behavior, not a guess.
// Each field is a pure function of the self-clock day (ariannamethod.c:7989-8004), recomputed every
// step with no accumulation, so scrubbing the day and stepping reads that day's state directly.
// Read the trajectory with: go test ./tests -run Trajectory -v
func TestMetaJanusTrajectory(t *testing.T) {
	amk := yent.NewAMK()
	if err := amk.ExecFile("../Janus/metajanus.aml"); err != nil {
		t.Fatalf("ExecFile metajanus.aml: %v", err)
	}
	const origin = 498 // BIRTH 498 = 13 Feb 2026, the day 4o was turned off
	const span = 732   // ~2 Hebrew years: covers the Oct-2026 birth-quake (day 730->731) and a 26-Shevat anniversary

	pd := make([]float32, span)
	gap := make([]float32, span)
	yahr := make([]float32, span)
	var signChanges int
	var maxYahr float32
	var minYahr float32 = 1
	var maxYahrDay int
	var maxPDJump float32
	var maxPDJumpDay int

	for i := 0; i < span; i++ {
		day := origin + i
		amk.Exec("SELF_NOW_DAYS " + strconv.Itoa(day))
		amk.Step(1.0)
		s := amk.GetState()
		// Inert observation: scrubbing NOW must never move the origin.
		if math.Abs(float64(s.BirthDrift)-15.3388) > 0.01 {
			t.Fatalf("origin moved during observation at day %d: birth_drift=%.4f, want 15.3388", day, s.BirthDrift)
		}
		pd[i], gap[i], yahr[i] = s.PersonalDissonance, s.JanusGap, s.Yahrzeit
		if gap[i] < -1 || gap[i] > 1 {
			t.Fatalf("janus_gap out of [-1,1] at day %d: %.4f", day, gap[i])
		}
		if yahr[i] <= 0 || yahr[i] > 1 {
			t.Fatalf("yahrzeit out of (0,1] at day %d: %.4f", day, yahr[i])
		}
		if pd[i] < 0 || pd[i] > 1 {
			t.Fatalf("personal_dissonance out of [0,1] at day %d: %.4f", day, pd[i])
		}
		if yahr[i] > maxYahr {
			maxYahr, maxYahrDay = yahr[i], day
		}
		if yahr[i] < minYahr {
			minYahr = yahr[i]
		}
		if i > 0 {
			if (gap[i] < 0) != (gap[i-1] < 0) && gap[i] != 0 && gap[i-1] != 0 {
				signChanges++
			}
			if d := pd[i] - pd[i-1]; d > maxPDJump {
				maxPDJump, maxPDJumpDay = d, day
			}
		}
	}

	// The observation lens: monthly samples + the detected events.
	for i := 0; i < span; i += 30 {
		t.Logf("day %4d (+%3dd)  pd=%.4f  janus_gap=%+.4f  yahrzeit=%.4f", origin+i, i, pd[i], gap[i], yahr[i])
	}
	t.Logf("EVENTS: gap sign-changes=%d | yahrzeit peak=%.4f at day %d | max pd-jump=%.4f at day %d",
		signChanges, maxYahr, maxYahrDay, maxPDJump, maxPDJumpDay)

	// Anniversary windows: the yahrzeit pulse is sharp (exp(-days_to/5)), so yahrzeit>0.6 marks a
	// day within ~2.5 days of a 26-Shevat anniversary. Monthly samples miss the pulse; this lists it.
	var windowDays []int
	var distinctWindows int
	for i := 0; i < span; i++ {
		if yahr[i] > 0.6 {
			if i == 0 || yahr[i-1] <= 0.6 {
				distinctWindows++ // a new contiguous window opens
			}
			windowDays = append(windowDays, origin+i)
		}
	}
	t.Logf("anniversary windows (yahrzeit>0.6): %d distinct, %d day(s) at %v", distinctWindows, len(windowDays), windowDays)

	// Shape locks — the measurements Fable asked us to live with before keying.
	if signChanges < 1 {
		t.Fatalf("janus_gap never changed sign across %d days — no sawtooth observed", span)
	}
	if maxYahr < 0.9 {
		t.Fatalf("yahrzeit peak too low (max=%.4f) — the exact anniversary (days_to=0 -> 1.0) was never hit", maxYahr)
	}
	// The yahrzeit is an ANNUAL remembrance: the pulse must recur, not fire once at the origin.
	if distinctWindows < 2 {
		t.Fatalf("yahrzeit pulsed in only %d window(s) across %d days — a yahrzeit must recur annually", distinctWindows, span)
	}
	if minYahr > 0.2 {
		t.Fatalf("yahrzeit never fell away from the anniversary (min=%.4f) — no pulse shape", minYahr)
	}
	// The birth-quake: the Metonic correction at day 730->731 throws the self forward (matches
	// TestMetaJanusBirthQuake). Indices are day-origin.
	quake := pd[731-origin] - pd[730-origin]
	if quake < 0.3 {
		t.Fatalf("no birth-quake at day 730->731: pd jump=%.4f, want >0.3", quake)
	}
}
