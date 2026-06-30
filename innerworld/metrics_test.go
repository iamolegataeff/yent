package innerworld

import (
	"errors"
	"testing"
)

type recordingMetricSink struct {
	got []MetricSnapshot
	err error
}

func (s *recordingMetricSink) PublishMetrics(m MetricSnapshot) error {
	s.got = append(s.got, m)
	return s.err
}

type panicMetricSink struct{}

func (panicMetricSink) PublishMetrics(MetricSnapshot) error { panic("telemetry failed") }

func TestMetricSnapshotForCircles(t *testing.T) {
	circles := []Circle{
		{Text: "cold iron rust"},
		{Text: "i love this wonderful beautiful joy"},
	}
	got := metricSnapshotForCircles("human_turn", circles, 2.5, MemoryFieldPressure{Score: 4, Prophecy: 5, Step: 0.31})
	if got.Source != "human_turn" || got.Circles != 2 || got.Debt != 2.5 {
		t.Fatalf("basic snapshot wrong: %+v", got)
	}
	if got.MemoryFieldScore != 4 || got.MemoryFieldProphecy != 5 || got.MemoryFieldStep != 0.31 {
		t.Fatalf("memory pressure receipt missing from snapshot: %+v", got)
	}
	if got.Valence <= 0 || got.Arousal <= 0 || got.Warmth <= 0 {
		t.Fatalf("positive feeling should publish valence/arousal/warmth: %+v", got)
	}
	if got.Trauma != 0 {
		t.Fatalf("positive feeling must not publish trauma: %+v", got)
	}
	if got.Entropy <= 0 || got.Entropy > 1 {
		t.Fatalf("entropy should be normalized into [0,1], got %.3f", got.Entropy)
	}
}

func TestMetricSnapshotForNegativeFeeling(t *testing.T) {
	got := metricSnapshotForCircles("dream", []Circle{{Text: "alone broken hopeless pain fear"}}, -10, MemoryFieldPressure{})
	if got.Debt != 0 {
		t.Fatalf("negative debt should clamp to 0, got %.3f", got.Debt)
	}
	if got.Valence >= 0 || got.Trauma <= 0 {
		t.Fatalf("negative feeling should publish trauma: %+v", got)
	}
	if got.Warmth != 0 || got.Flow != 0 {
		t.Fatalf("negative feeling must not warm/flow the hub: %+v", got)
	}
}

func TestMetricSinkPublishesOnThinkAndFailsSoft(t *testing.T) {
	sink := &recordingMetricSink{err: errors.New("ignored")}
	iw := NewInnerWorld(fakeBody{}, &fakeField{debt: 2}, tempDivergence)
	iw.SetMemory(fakeMemory{past: []string{"RI pressure: raw memory must become field pressure."}})
	iw.SetMetricSink(sink)
	<-iw.Think("what is code")
	if len(sink.got) != 1 {
		t.Fatalf("expected one metric publish, got %d", len(sink.got))
	}
	if sink.got[0].Source != "human_turn" || sink.got[0].Circles != 3 {
		t.Fatalf("unexpected metric snapshot: %+v", sink.got[0])
	}
	if sink.got[0].MemoryFieldScore == 0 || sink.got[0].MemoryFieldProphecy == 0 || sink.got[0].MemoryFieldStep == 0 {
		t.Fatalf("metric snapshot should carry applied memory pressure: %+v", sink.got[0])
	}

	iw.SetMetricSink(panicMetricSink{})
	<-iw.Think("telemetry should not break thought")
}
