package tests

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

const amSomaMagic = 0x4F534D41 // 'A','M','S','O' little-endian

// craftSoma writes a soma file with the given version and state_sz and a zero-filled state region.
func craftSoma(t *testing.T, path string, version, stateSz uint32) {
	t.Helper()
	hdr := make([]byte, 20)
	binary.LittleEndian.PutUint32(hdr[0:], amSomaMagic)
	binary.LittleEndian.PutUint32(hdr[4:], version)
	binary.LittleEndian.PutUint32(hdr[8:], stateSz)
	// timestamp bytes [12:20] left zero
	buf := append(hdr, make([]byte, stateSz)...)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write craft soma: %v", err)
	}
}

// A-1 (Fable audit): the soma gate accepts any older soma as a PREFIX (state_sz in (0, PERSIST_SZ]),
// so appending fields to AM_State never again orphans existing somas — the old exact-size gate refused
// every pre-append file. A state_sz larger than the current region (newer layout / junk) is refused.
func TestMetaJanusSomaPrefixLoad(t *testing.T) {
	dir := t.TempDir()

	// Learn PERSIST_SZ from a real save — it is exactly the state_sz the kernel writes.
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
	persistSz := binary.LittleEndian.Uint32(raw[8:12])
	if persistSz == 0 {
		t.Fatalf("real soma state_sz is 0")
	}

	// Accepted prefixes: current region, pre-b4 layout (P-8), true v2 (P-20, before positive-soma).
	accept := []struct {
		name    string
		version uint32
		sz      uint32
	}{
		{"P_current_v3", 3, persistSz},
		{"P-8_pre-b4_v3", 3, persistSz - 8},
		{"P-20_v2", 2, persistSz - 20},
	}
	for _, c := range accept {
		p := filepath.Join(dir, c.name+".soma")
		craftSoma(t, p, c.version, c.sz)

		b := yent.NewAMK()
		b.Exec("PROPHECY 7") // field weather an accepted load must wipe (memset of the region)
		if got := b.GetState().Prophecy; got != 7 {
			t.Fatalf("%s: setup PROPHECY 7 -> %d", c.name, got)
		}
		if err := b.Exec(`LOAD "` + p + `"`); err != nil {
			t.Fatalf("%s: LOAD err: %v", c.name, err)
		}
		s := b.GetState()
		if s.Prophecy != 0 {
			t.Errorf("%s: LOAD not accepted — prophecy = %d, want 0 (weather memset-zeroed)", c.name, s.Prophecy)
		}
		if s.BirthDrift != 0 {
			t.Errorf("%s: LOAD injected an origin — BirthDrift = %.4f, want 0", c.name, s.BirthDrift)
		}
	}

	// Refused: state_sz larger than the current region (a newer layout we cannot interpret).
	p := filepath.Join(dir, "P+4.soma")
	craftSoma(t, p, 3, persistSz+4)
	c := yent.NewAMK()
	c.Exec("PROPHECY 7")
	c.Exec(`LOAD "` + p + `"`) // the LOAD op ignores rc; a refusal must leave G untouched
	if got := c.GetState().Prophecy; got != 7 {
		t.Errorf("P+4 (>PERSIST_SZ) was accepted — prophecy = %d, want 7 (load refused, G untouched)", got)
	}
}
