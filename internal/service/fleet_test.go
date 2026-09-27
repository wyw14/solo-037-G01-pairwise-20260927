package service

import (
	"testing"

	"github.com/wyw14/solo-037/internal/model"
)

func fleetParams(draft float64) model.ProfileParams {
	return model.ProfileParams{
		Geometry: model.ProfileGeometry{LengthMeters: 4, BeamMeters: 0.6, DepthMeters: 0.35, DraftMeters: draft},
		Load:     model.ProfileLoad{DisplacementKilograms: 180, LoadCondition: "full_load"},
		Installation: model.ProfileInstallation{
			TowPoint:   "bow",
			MountType:  "strut",
			Appendages: []string{"rudder"},
		},
	}
}

func TestFleetArchiveVersionsPersistAcrossReopen(t *testing.T) {
	directory := t.TempDir()
	a, err := OpenFleetArchive(directory)
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.RecordVersion("hull A", fleetParams(0.2), "initial")
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.RecordVersion("hull A", fleetParams(0.25), "deeper ballast")
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || second.Version != 2 {
		t.Fatalf("unexpected versions: %v %v", first, second)
	}
	loaded, err := OpenFleetArchive(directory)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := loaded.Get("hull A")
	if !ok || len(profile.Versions) != 2 {
		t.Fatalf("versions not persisted: %+v", profile)
	}
	current, ok := loaded.Current("hull A")
	if !ok || current.Version != 2 || current.Params.Geometry.DraftMeters != 0.25 {
		t.Fatalf("current version wrong after reload: %+v", current)
	}
	retained, ok := profile.Version(first.Revision)
	if !ok || retained.Params.Geometry.DraftMeters != 0.2 {
		t.Fatalf("old version not retained after reload: %+v", retained)
	}
	if _, err = loaded.RecordVersion("hull A", model.ProfileParams{}, "bad"); err == nil {
		t.Fatal("invalid params accepted")
	}
	if _, err = loaded.RecordVersion("  ", fleetParams(0.2), ""); err == nil {
		t.Fatal("empty model name accepted")
	}
	if len(loaded.List()) != 1 {
		t.Fatalf("rejected records changed archive: %+v", loaded.List())
	}
}

func TestSectionArchiveRecordAndReload(t *testing.T) {
	directory := t.TempDir()
	a, err := OpenSectionArchive(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Record(model.SectionCondition{Section: "section-a", DepthMeters: 2.4, WidthMeters: 6}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Record(model.SectionCondition{Section: "section-a", DepthMeters: 0}); err == nil {
		t.Fatal("invalid condition accepted")
	}
	loaded, err := OpenSectionArchive(directory)
	if err != nil {
		t.Fatal(err)
	}
	condition, ok := loaded.Get("section-a")
	if !ok || condition.DepthMeters != 2.4 || condition.WidthMeters != 6 || condition.Revision == "" {
		t.Fatalf("condition not persisted: %+v", condition)
	}
	if len(loaded.List()) != 1 {
		t.Fatalf("unexpected conditions: %+v", loaded.List())
	}
}

func TestMeasuredRunPinsProfileVersionAcrossReconfiguration(t *testing.T) {
	directory := t.TempDir()
	runtime, err := OpenRuntime(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.RecordModelVersion("hull A", fleetParams(0.2), "initial"); err != nil {
		t.Fatal(err)
	}
	firstRun, err := runtime.CreateMeasuredRun("tow 1", "hull A", "section-a", "carriage-1")
	if err != nil {
		t.Fatal(err)
	}
	if firstRun.Profile == nil || firstRun.Profile.Version != 1 {
		t.Fatalf("run not pinned to version 1: %+v", firstRun.Profile)
	}
	aggressive := fleetParams(2.5)
	aggressive.Geometry.DepthMeters = 3
	if _, err = runtime.RecordModelVersion("hull A", aggressive, "aggressive ballast"); err != nil {
		t.Fatal(err)
	}
	secondRun, err := runtime.CreateMeasuredRun("tow 2", "hull A", "section-a", "carriage-1")
	if err != nil {
		t.Fatal(err)
	}
	if secondRun.Profile == nil || secondRun.Profile.Version != 2 {
		t.Fatalf("new run not pinned to version 2: %+v", secondRun.Profile)
	}
	if _, err = runtime.RecordSectionCondition(model.SectionCondition{Section: "section-a", DepthMeters: 2.4, WidthMeters: 6}); err != nil {
		t.Fatal(err)
	}

	// Reload everything from disk: the pin and the version history must survive.
	runtime, err = OpenRuntime(directory)
	if err != nil {
		t.Fatal(err)
	}
	firstReport, err := runtime.EvaluateRunCompatibility(firstRun.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstReport.Pin.Version != 1 || firstReport.Verdict != model.VerdictCompatible {
		t.Fatalf("first run not evaluated against its pinned version: %+v", firstReport)
	}
	secondReport, err := runtime.EvaluateRunCompatibility(secondRun.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondReport.Pin.Version != 2 || secondReport.Verdict != model.VerdictIncompatible {
		t.Fatalf("second run should ground with its pinned draft: %+v", secondReport)
	}
}

func TestEvaluateRunCompatibilityRequiresPinnedProfile(t *testing.T) {
	runtime, err := OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run, err := runtime.CreateMeasuredRun("tow 1", "unregistered hull", "section-a", "carriage-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Profile != nil {
		t.Fatalf("unregistered model pinned: %+v", run.Profile)
	}
	if _, err = runtime.EvaluateRunCompatibility(run.Operation.ID); err == nil {
		t.Fatal("evaluation without pinned profile accepted")
	}
	if _, err = runtime.EvaluateRunCompatibility("missing"); err == nil {
		t.Fatal("evaluation for unknown run accepted")
	}
}
