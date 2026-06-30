package innerworld

import "testing"

// fakeSense is a fixed environment perception for the tests.
type fakeSense struct {
	aml string
	ok  bool
}

func (s fakeSense) Pressure() (string, bool) { return s.aml, s.ok }

func TestApplySenseDrivesField(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.SetSense(fakeSense{aml: "VELOCITY RUN\nPROPHECY 7", ok: true})

	iw.genMu.Lock()
	iw.applySenseLocked()
	iw.genMu.Unlock()

	got := f.scriptList()
	if len(got) != 2 || got[0] != "VELOCITY RUN" || got[1] != "PROPHECY 7" {
		t.Errorf("sense should exec each AML line into the field, got %v", got)
	}
	if f.steps == 0 {
		t.Error("sense should settle the field one step after the reflex")
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

// the present reflex skips blank lines and stops fail-soft, but a well-formed
// perception lands fully.
func TestApplySenseSkipsBlankLines(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.SetSense(fakeSense{aml: "VELOCITY NOMOVE\n\n  \nPROPHECY 1", ok: true})

	iw.genMu.Lock()
	iw.applySenseLocked()
	iw.genMu.Unlock()

	got := f.scriptList()
	if len(got) != 2 || got[0] != "VELOCITY NOMOVE" || got[1] != "PROPHECY 1" {
		t.Errorf("blank lines should be skipped, got %v", got)
	}
}
