package innerworld

import "strings"

// Sense is the organism's perception of the world around the inference engine — the
// body (SARTRE) feeling its environment. Where Memory pulls the PAST in as field
// pressure (slow, experience), Sense pushes the PRESENT in as a reflex: the
// environment shifts the AML field's posture BEFORE the circles rise. SARTRE's
// perception already speaks AML (VELOCITY/PROPHECY natively, sartre/perception.c),
// so the reflex is one physics, not a translation. nil = the organism feels no
// environment (backward-compatible).
type Sense interface {
	// Pressure returns the environment's current AML field commands — one per line,
	// e.g. "VELOCITY RUN\nPROPHECY 7" — and whether there is anything to feel right
	// now. A quiet environment returns ok=false (no pulse).
	Pressure() (aml string, ok bool)
}

type ackingSense interface {
	AckPressure() error
}

// senseStep is how hard the environment settles the field after its reflex — a nudge,
// not a lurch (the world tilts posture; it does not seize the organism).
const senseStep = 0.15

// SetSense wires the environment perception. With it, the field takes the posture of
// the present world before each ripple — a fast reflex complementary to Memory's
// slower recall pressure. Set before Think/Breathe start.
func (iw *InnerWorld) SetSense(s Sense) {
	iw.genMu.Lock()
	iw.sense = s
	iw.genMu.Unlock()
}

// applySenseLocked lets the environment shift the field's posture before the circles
// rise — the fast sensory reflex, the present-time twin of applyMemoryPressureLocked's
// slow recall pressure. The perception is already AML, so the reflex is applied as
// one script and then the field settles one small step. Caller holds genMu; the Field
// owns its own locking. Fail-soft: a nil sense/field, nothing to feel, or a rejected
// script never stops thought. NO-SEED holds — this is a field command, never seed text.
func (iw *InnerWorld) applySenseLocked() {
	if iw.sense == nil || iw.field == nil {
		return
	}
	aml, ok := iw.sense.Pressure()
	if !ok {
		return
	}
	script := compactSenseAML(aml)
	if script == "" {
		return
	}
	if err := iw.field.Exec(script); err != nil {
		return
	}
	iw.field.Step(senseStep)
	if ack, ok := iw.sense.(ackingSense); ok {
		_ = ack.AckPressure()
	}
}

func compactSenseAML(aml string) string {
	lines := make([]string, 0, 2)
	for _, line := range strings.Split(aml, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
