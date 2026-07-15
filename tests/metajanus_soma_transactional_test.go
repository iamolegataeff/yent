package tests

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// MED-2 (Sol audit): am_field_load zeroed the live field weather BEFORE it knew the payload was complete,
// so a truncated soma destroyed the live state and returned an error the AML LOAD then swallowed. The fix
// reads into a temp buffer and commits to G only after a full read — a truncated load refuses without
// touching the live field.
func TestMetaJanusTruncatedSomaKeepsLiveField(t *testing.T) {
	dir := t.TempDir()

	// Learn the real state_sz from a valid save.
	real := filepath.Join(dir, "real.soma")
	a := yent.NewAMK()
	a.Exec("BIRTH 498")
	a.Exec("PROPHECY 7")
	a.Step(1.0)
	if err := a.Exec(`SAVE "` + real + `"`); err != nil {
		t.Fatalf("SAVE: %v", err)
	}
	raw, err := os.ReadFile(real)
	if err != nil {
		t.Fatalf("read real soma: %v", err)
	}
	stateSz := binary.LittleEndian.Uint32(raw[8:12])

	// A truncated soma: the 20-byte header claims the full state_sz, but the payload is cut in half.
	trunc := filepath.Join(dir, "trunc.soma")
	cut := 20 + int(stateSz)/2
	if err := os.WriteFile(trunc, raw[:cut], 0o644); err != nil {
		t.Fatalf("write truncated soma: %v", err)
	}

	// A fresh live field with a distinctive, non-zero value.
	b := yent.NewAMK()
	b.Exec("BIRTH 498")
	b.Exec("PROPHECY 9")
	b.Step(1.0)
	before := b.GetState().Prophecy
	if before == 0 {
		t.Fatalf("setup: live prophecy is 0, cannot detect zeroing")
	}
	// Loading a truncated soma must REFUSE without destroying the live field.
	b.Exec(`LOAD "` + trunc + `"`) // the AML LOAD swallows the result; check the side effect
	if after := b.GetState().Prophecy; after != before {
		t.Fatalf("truncated soma changed the live field: prophecy %d -> %d (must stay intact)", before, after)
	}
}
