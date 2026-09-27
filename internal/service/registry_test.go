package service

import (
	"math"
	"testing"
)

func TestModelRegistryPersistsVersionsAcrossReload(t *testing.T) {
	directory := t.TempDir()
	registry, err := OpenModelRegistry(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.RegisterModel("hull-x", 0.15, 3.2, 0.5, 120, "bare hull", "initial"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.RegisterModel("hull-x", 0.15, 3.2, 0.5, 120, "bare hull", ""); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	if _, err := registry.AddConfig("hull-x", 0.18, 3.2, 0.5, 135, "with appendages", "ballast change"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AddConfig("ghost", 0.1, 1, 0.2, 10, "", ""); err == nil {
		t.Fatal("config accepted for unknown model")
	}
	if _, err := registry.SetSectionCondition("section-a", 1.4, "after refill"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenModelRegistry(directory)
	if err != nil {
		t.Fatal(err)
	}
	ship, ok := reloaded.Model("hull-x")
	if !ok {
		t.Fatal("model missing after reload")
	}
	if len(ship.Versions) != 2 || ship.Versions[0].DraftMeters != 0.15 || ship.Versions[1].DraftMeters != 0.18 {
		t.Fatalf("versions not preserved: %+v", ship.Versions)
	}
	current, ok := reloaded.CurrentConfig("hull-x")
	if !ok || current.Version != 2 || current.Installation != "with appendages" {
		t.Fatalf("current config wrong after reload: %+v", current)
	}
	conditions := reloaded.SectionConditions()
	if len(conditions) != 1 || conditions[0].DepthMeters != 1.4 {
		t.Fatalf("section condition missing after reload: %+v", conditions)
	}
}

func TestModelRegistryRejectsInvalidGeometry(t *testing.T) {
	registry, err := OpenModelRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.RegisterModel("", 0.15, 3.2, 0.5, 120, "", ""); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := registry.RegisterModel("hull-y", 0, 3.2, 0.5, 120, "", ""); err == nil {
		t.Fatal("zero draft accepted")
	}
	if _, err := registry.RegisterModel("hull-y", math.NaN(), 3.2, 0.5, 120, "", ""); err == nil {
		t.Fatal("NaN draft accepted")
	}
	if _, err := registry.RegisterModel("hull-y", 0.15, 3.2, 0.5, -1, "", ""); err == nil {
		t.Fatal("negative displacement accepted")
	}
	if _, err := registry.SetSectionCondition("section-a", -0.5, ""); err == nil {
		t.Fatal("negative depth accepted")
	}
	if len(registry.Models()) != 0 || len(registry.SectionConditions()) != 0 {
		t.Fatal("rejected inputs changed registry")
	}
}

func TestCompatibilityKeepsPinnedVersionAfterConfigChange(t *testing.T) {
	runtime, err := OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RegisterShipModel("hull-x", 0.4, 3.2, 0.5, 120, "bare hull", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SetSectionCondition("section-a", 1.0, ""); err != nil {
		t.Fatal(err)
	}
	first, err := runtime.CreateMeasuredRun("tow 1", "hull-x", "section-a", "carriage-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ModelConfig == nil || first.ModelConfig.Version != 1 {
		t.Fatalf("run did not pin version 1: %+v", first.ModelConfig)
	}
	report, err := runtime.EvaluateRunCompatibility(first.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != CompatibilityCompatible || report.ConfigVersion == nil || *report.ConfigVersion != 1 {
		t.Fatalf("expected compatible at version 1: %+v", report)
	}
	// Deepen the draft: clearance drops from 0.6 m to 0.4 m, below the 0.5 m minimum.
	if _, err := runtime.AddShipModelConfig("hull-x", 0.6, 3.2, 0.5, 150, "loaded", "deeper ballast"); err != nil {
		t.Fatal(err)
	}
	historical, err := runtime.EvaluateRunCompatibility(first.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Status != CompatibilityCompatible || *historical.ConfigVersion != 1 {
		t.Fatalf("historical run must keep version 1 evaluation: %+v", historical)
	}
	second, err := runtime.CreateMeasuredRun("tow 2", "hull-x", "section-a", "carriage-1")
	if err != nil {
		t.Fatal(err)
	}
	if second.ModelConfig == nil || second.ModelConfig.Version != 2 {
		t.Fatalf("new run did not pin version 2: %+v", second.ModelConfig)
	}
	current, err := runtime.EvaluateRunCompatibility(second.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != CompatibilityIncompatible {
		t.Fatalf("expected incompatible for deep draft: %+v", current)
	}
	var clearance *CompatibilityCheck
	for i := range current.Checks {
		if current.Checks[i].Name == "underkeel_clearance" {
			clearance = &current.Checks[i]
		}
	}
	if clearance == nil || clearance.Status != checkFail || math.Abs(clearance.Value-0.4) > 1e-9 || clearance.Limit != MinUnderkeelClearanceMeters {
		t.Fatalf("clearance check wrong: %+v", clearance)
	}
}

func TestCompatibilityUnknownWithoutConfigOrCondition(t *testing.T) {
	runtime, err := OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	unregistered, err := runtime.CreateMeasuredRun("tow ghost", "ghost", "section-a", "carriage-1")
	if err != nil {
		t.Fatal(err)
	}
	if unregistered.ModelConfig != nil {
		t.Fatal("unregistered model should not pin a config")
	}
	report, err := runtime.EvaluateRunCompatibility(unregistered.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != CompatibilityUnknown {
		t.Fatalf("expected unknown without pinned config: %+v", report)
	}
	if _, err := runtime.RegisterShipModel("hull-x", 0.4, 3.2, 0.5, 120, "", ""); err != nil {
		t.Fatal(err)
	}
	dry, err := runtime.CreateMeasuredRun("tow dry", "hull-x", "section-nowhere", "carriage-1")
	if err != nil {
		t.Fatal(err)
	}
	report, err = runtime.EvaluateRunCompatibility(dry.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != CompatibilityUnknown || report.SectionDepth != nil {
		t.Fatalf("expected unknown without section condition: %+v", report)
	}
	if _, err := runtime.EvaluateRunCompatibility("missing"); err == nil {
		t.Fatal("evaluation accepted unknown run")
	}
}
