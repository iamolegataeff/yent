package innerworld

// MetricSnapshot is the inner world's current field weather, formatted for
// telemetry sinks such as SARTRE. It is not prompt text and must not be fed back
// as dialogue. Values are bounded so a broken thought cannot poison the hub.
type MetricSnapshot struct {
	Source              string
	Circles             int
	Debt                float32
	Coherence           float32
	Entropy             float32
	Valence             float32
	Arousal             float32
	Trauma              float32
	Warmth              float32
	Flow                float32
	MemoryFieldScore    float32
	MemoryFieldProphecy float32
	MemoryFieldStep     float32
}

// MetricSink receives field-weather snapshots. Implementations are telemetry
// edges; they must not be required for thought to continue.
type MetricSink interface {
	PublishMetrics(MetricSnapshot) error
}

// SetMetricSink wires a telemetry sink for the inner field weather. nil disables
// the bridge. Set before Think/Breathe start.
func (iw *InnerWorld) SetMetricSink(s MetricSink) {
	iw.genMu.Lock()
	iw.metricSink = s
	iw.genMu.Unlock()
}

func metricSnapshotForCircles(source string, circles []Circle, debt float32, memoryPressure MemoryFieldPressure) MetricSnapshot {
	s := MetricSnapshot{
		Source:              source,
		Circles:             len(circles),
		Debt:                nonNegativeFinite(debt),
		MemoryFieldScore:    nonNegativeFinite(float32(memoryPressure.Score)),
		MemoryFieldProphecy: nonNegativeFinite(float32(memoryPressure.Prophecy)),
		MemoryFieldStep:     nonNegativeFinite(memoryPressure.Step),
	}
	if len(circles) == 0 {
		return s
	}
	last := circles[len(circles)-1].Text
	valence, _ := feelText(last)
	entropyRaw := feelEntropy(last)
	arousal := entropyRaw / (entropyRaw + 1)
	var coherence float32
	if len(circles) >= 2 {
		coherence = feelResonance(last, circles[len(circles)-2].Text)
	}

	s.Coherence = clamp01(finite(coherence))
	s.Entropy = clamp01(finite(arousal))
	s.Valence = clampSigned01(finite(valence))
	s.Arousal = clamp01(finite(arousal))
	switch {
	case s.Valence >= feelThreshold:
		s.Warmth = s.Valence
		s.Flow = s.Coherence
	case s.Valence <= -feelThreshold:
		s.Trauma = -s.Valence
	}
	return s
}

func (iw *InnerWorld) publishMetricsLocked(source string, circles []Circle, debt float32, memoryPressure MemoryFieldPressure) {
	if iw.metricSink == nil {
		return
	}
	snapshot := metricSnapshotForCircles(source, circles, debt, memoryPressure)
	defer func() { _ = recover() }() // telemetry must not take the inner voice down
	_ = iw.metricSink.PublishMetrics(snapshot)
}

func nonNegativeFinite(v float32) float32 {
	v = finite(v)
	if v < 0 {
		return 0
	}
	return v
}

func clampSigned01(v float32) float32 {
	if v != v {
		return 0
	}
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}
