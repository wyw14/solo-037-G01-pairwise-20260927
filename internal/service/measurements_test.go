package service

import (
	"math"
	"testing"
	"time"

	"github.com/wyw14/solo-037/internal/model"
)

func TestOperatorMeasurementsPersistAndAnalyzeSuppliedValues(t *testing.T) {
	directory := t.TempDir()
	a, err := OpenMeasurementArchive(directory)
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.Create("test A", "hull X", "section-a", "carriage-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Minute).UTC()
	for _, input := range []MeasurementInput{
		{Kind: model.EvidenceWaveTime, SourceID: "left", Value: 0.1, RecordedAt: at},
		{Kind: model.EvidenceWaveTime, SourceID: "right", Value: 0.3, RecordedAt: at.Add(time.Second)},
		{Kind: model.EvidenceForce, SourceID: "load-cell", Value: 12, RecordedAt: at},
	} {
		if _, err := a.Record(run.Operation.ID, input); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := OpenMeasurementArchive(directory)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := loaded.Analyze(run.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Wave.Count != 2 || math.Abs(analysis.Wave.Mean-0.2) > 1e-9 || analysis.ForceCount != 1 || analysis.ForceMean != 12 {
		t.Fatalf("analysis did not reflect supplied values after reload: %+v", analysis)
	}
	if len(loaded.List()) != 1 || len(loaded.List()[0].Evidence) != 3 || !loaded.List()[0].Evidence[0].Durable {
		t.Fatalf("persisted evidence missing: %+v", loaded.List())
	}
}

func TestOperatorMeasurementRejectsInvalidAndUnknownRun(t *testing.T) {
	a, err := OpenMeasurementArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.Create("test B", "hull X", "section-a", "carriage-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	input := MeasurementInput{Kind: model.EvidenceWaveTime, SourceID: "left", Value: 1, RecordedAt: time.Now().UTC()}
	if _, err := a.Record("missing", input); err == nil {
		t.Fatal("unknown run accepted")
	}
	input.Value = math.NaN()
	if _, err := a.Record(run.Operation.ID, input); err == nil {
		t.Fatal("NaN accepted")
	}
	if len(a.List()[0].Evidence) != 0 {
		t.Fatal("rejected values changed archive")
	}
}
