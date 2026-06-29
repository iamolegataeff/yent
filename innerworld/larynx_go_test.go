package innerworld

import "testing"

func TestTextureLarynx(t *testing.T) {
	flowing := []Circle{{Text: "the sea is not the ocean but a heartbeat voice rippling"}}
	looping := []Circle{{Text: "loop loop loop loop loop loop loop loop"}}

	var lx textureLarynx
	cf := lx.Couple(flowing)
	cl := lx.Couple(looping)

	if cf <= cl {
		t.Errorf("a flowing thought should couple more than a loop: flowing=%.3f looping=%.3f", cf, cl)
	}
	for _, c := range []float32{cf, cl, lx.Couple(nil)} {
		if c < 0 || c > 1 {
			t.Errorf("coupling out of [0,1]: %.3f", c)
		}
	}
}
