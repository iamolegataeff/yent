package innerworld

import "testing"

// fakeSense is a fixed environment perception for the tests.
type fakeSense struct {
	aml string
	ok  bool
}

func (s fakeSense) Pressure() (string, bool) { return s.aml, s.ok }

type fakeAckSense struct {
	aml  string
	ok   bool
	acks int
}

func (s *fakeAckSense) Pressure() (string, bool) { return s.aml, s.ok }

func (s *fakeAckSense) AckPressure() error {
	s.acks++
	return nil
}

func TestApplySenseDrivesField(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.SetSense(fakeSense{aml: "VELOCITY RUN\nPROPHECY 7", ok: true})

	iw.genMu.Lock()
	iw.applySenseLocked()
	iw.genMu.Unlock()

	got := f.scriptList()
	if len(got) != 1 || got[0] != "VELOCITY RUN\nPROPHECY 7" {
		t.Errorf("sense should exec one atomic AML block into the field, got %v", got)
	}
	if f.steps == 0 {
		t.Error("sense should settle the field one step after the reflex")
	}
}

func TestApplySenseAcksAfterFieldStep(t *testing.T) {
	f := &fakeField{}
	sense := &fakeAckSense{aml: "VELOCITY RUN\nPROPHECY 7", ok: true}
	iw := NewInnerWorld(nil, f, nil)
	iw.SetSense(sense)

	iw.genMu.Lock()
	iw.applySenseLocked()
	iw.genMu.Unlock()

	if sense.acks != 1 {
		t.Fatalf("sense pressure should be acked once after field execution, got %d", sense.acks)
	}
}

func TestApplySenseQuietIsNoop(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.SetSense(fakeSense{aml: "", ok: false}) // a quiet world feels nothing

	iw.genMu.Lock()
	iw.applySenseLocked()
	iw.genMu.Unlock()

	if len(f.scriptList()) != 0 || f.steps != 0 {
		t.Error("a quiet environment must not touch the field")
	}
}

func TestApplySenseNilSafe(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil) // no sense wired

	iw.genMu.Lock()
	iw.applySenseLocked()
	iw.genMu.Unlock()

	if len(f.scriptList()) != 0 || f.steps != 0 {
		t.Error("nil sense must be a no-op")
	}
}

// the present reflex skips blank lines and lands a well-formed perception as one block.
func TestApplySenseSkipsBlankLines(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.SetSense(fakeSense{aml: "VELOCITY NOMOVE\n\n  \nPROPHECY 1", ok: true})

	iw.genMu.Lock()
	iw.applySenseLocked()
	iw.genMu.Unlock()

	got := f.scriptList()
	if len(got) != 1 || got[0] != "VELOCITY NOMOVE\nPROPHECY 1" {
		t.Errorf("blank lines should be skipped, got %v", got)
	}
}

func TestApplySenseRejectedBlockDoesNotStep(t *testing.T) {
	f := &fakeField{failOn: "PROPHECY"}
	sense := &fakeAckSense{aml: "VELOCITY RUN\nPROPHECY 7", ok: true}
	iw := NewInnerWorld(nil, f, nil)
	iw.SetSense(sense)

	iw.genMu.Lock()
	iw.applySenseLocked()
	iw.genMu.Unlock()

	if len(f.scriptList()) != 0 || f.steps != 0 {
		t.Errorf("a rejected sense block must not partially apply or step, got scripts=%v steps=%d", f.scriptList(), f.steps)
	}
	if sense.acks != 0 {
		t.Fatalf("rejected sense block must not be acknowledged, got %d acks", sense.acks)
	}
}
