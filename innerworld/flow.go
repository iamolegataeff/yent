package innerworld

// Flow is Yent's third body — the resident AML organism that merges the two voices
// (nemo fast + small24 deep) into one "Я". It is the Field (the AML physics) PLUS
// the consolidation organs Kairos drives in sleep: the cooc memory, the scar sea,
// and the pressure the body puts back on a voice's logits. Two honest forms share
// this interface: goFlow (pure Go, for tests and kernel-less hosts) and, in
// production, the native AML body (am_cooc / SCAR / parliament) over cgo. Streams
// flow IN via Ingest; consolidation runs in sleep; the body pushes OUT via
// ApplyPressure.
type Flow interface {
	Field // Exec / Step / Debt / Destiny — the AML bridge

	// Ingest folds a thought (a circle or a deep answer) into the body's
	// co-occurrence memory — the stream entering the body.
	Ingest(text string)
	// ConsolidateCooc runs the seasonal cooc harvest; returns edges pruned.
	ConsolidateCooc() int

	// Scar sinks a rejected thought (one that broke prophecy-destiny coherence) into
	// the gravitational sea, with gravity proportional to how far it broke it.
	Scar(text string, gravity float32)
	// ConsolidateScar runs the scar sea's decay/prune; returns scars forgotten.
	ConsolidateScar() int

	// ApplyPressure lets the body push on a voice's logits (am_apply_field_to_logits
	// in the AML body). The Go fallback is a no-op — field-pressure is a real
	// AML/Metal feature, not faked here.
	ApplyPressure(logits []float32)

	// AutumnEnergy reports how ripe the field is for the harvest (0..1). Kairos uses
	// it for critical mass: high coherence drives the field into autumn, and autumn
	// is when consolidation lands.
	AutumnEnergy() float32
}

// goFlow is the pure-Go fallback body: the third body without cgo, wrapping the Go
// cooc graph, the scar sea, and the field. Same Flow interface as the native AML
// body, so Kairos and tests run identically; only ApplyPressure and the season are
// stubs the real AML body fills.
type goFlow struct {
	Field
	cooc *CoocGraph
	scar *ScarMemory

	coocReinforce float32
	coocFloor     float32
	scarFloor     float32
}

// NewGoFlow builds the pure-Go fallback body over a field, a cooc graph, and a scar
// sea (any may be nil — those organs simply no-op). The reinforce/floor knobs are
// the seasonal harvest strengths.
func NewGoFlow(field Field, cooc *CoocGraph, scar *ScarMemory, coocReinforce, coocFloor, scarFloor float32) *goFlow {
	return &goFlow{
		Field:         field,
		cooc:          cooc,
		scar:          scar,
		coocReinforce: coocReinforce,
		coocFloor:     coocFloor,
		scarFloor:     scarFloor,
	}
}

func (f *goFlow) Ingest(text string) {
	if f.cooc != nil {
		f.cooc.Observe(text)
	}
}

func (f *goFlow) ConsolidateCooc() int {
	if f.cooc == nil {
		return 0
	}
	return f.cooc.Consolidate(f.coocReinforce, f.coocFloor)
}

func (f *goFlow) Scar(text string, gravity float32) {
	if f.scar != nil {
		f.scar.Scar(text, gravity)
	}
}

func (f *goFlow) ConsolidateScar() int {
	if f.scar == nil {
		return 0
	}
	return f.scar.Consolidate(f.scarFloor)
}

func (f *goFlow) ApplyPressure([]float32) {
	// no token-level field in the Go fallback; the AML body applies
	// am_apply_field_to_logits. Honest no-op — pressure is a Metal/AML feature.
}

func (f *goFlow) AutumnEnergy() float32 {
	// no season in the Go fallback; the AML body reads G.autumn_energy. Stand in with
	// a saturating function of field debt so Kairos can still gate critical mass.
	if f.Field == nil {
		return 0
	}
	d := f.Field.Debt()
	if d <= 0 {
		return 0
	}
	return d / (d + 1)
}
