package model

import (
    "strings"
    "testing"
    "time"
)

func testParams() ProfileParams {
    return ProfileParams{
        Geometry: ProfileGeometry{LengthMeters: 4, BeamMeters: 0.6, DepthMeters: 0.35, DraftMeters: 0.2},
        Load:     ProfileLoad{DisplacementKilograms: 180, LoadCondition: "full_load"},
        Installation: ProfileInstallation{
            TowPoint:   "bow",
            MountType:  "strut",
            Appendages: []string{"rudder"},
        },
    }
}

func testPin() ProfilePin {
    return ProfilePin{ModelName: "hull-a", Revision: NewRevisionID(), Version: 1}
}

func checkStatus(report CompatibilityReport, name string) CheckStatus {
    for _, check := range report.Checks {
        if check.Name == name {
            return check.Status
        }
    }
    return ""
}

func TestProfileParamsValidate(t *testing.T) {
    if err := testParams().Validate(); err != nil {
        t.Fatalf("valid params rejected: %v", err)
    }
    cases := []ProfileParams{
        func() ProfileParams { p := testParams(); p.Geometry.DraftMeters = 0; return p }(),
        func() ProfileParams { p := testParams(); p.Geometry.BeamMeters = -1; return p }(),
        func() ProfileParams { p := testParams(); p.Geometry.DraftMeters = 0.4; return p }(),
        func() ProfileParams { p := testParams(); p.Load.DisplacementKilograms = -1; return p }(),
    }
    for index, params := range cases {
        if err := params.Validate(); err == nil {
            t.Fatalf("case %d accepted invalid params", index)
        }
    }
}

func TestProfileAppendKeepsImmutableHistory(t *testing.T) {
    profile := ShipModelProfile{Name: "hull-a"}
    profile, first, err := profile.Append(testParams(), "initial", time.Now())
    if err != nil {
        t.Fatal(err)
    }
    deeper := testParams()
    deeper.Geometry.DraftMeters = 0.25
    profile, second, err := profile.Append(deeper, "deeper ballast", time.Now())
    if err != nil {
        t.Fatal(err)
    }
    if first.Version != 1 || second.Version != 2 || first.Revision == second.Revision {
        t.Fatalf("unexpected versions: %v %v", first, second)
    }
    current, ok := profile.Current()
    if !ok || current.Version != 2 || current.Params.Geometry.DraftMeters != 0.25 {
        t.Fatalf("current version wrong: %+v", current)
    }
    retained, ok := profile.Version(first.Revision)
    if !ok || retained.Params.Geometry.DraftMeters != 0.2 {
        t.Fatalf("old version not retained: %+v", retained)
    }
    if _, _, err = profile.Append(ProfileParams{}, "bad", time.Now()); err == nil {
        t.Fatal("invalid params appended")
    }
}

func TestEvaluateCompatibilityWithoutSectionConditions(t *testing.T) {
    report := EvaluateCompatibility(testPin(), testParams(), SectionCondition{Section: "section-a"}, false, time.Now())
    if report.Verdict != VerdictUnknown {
        t.Fatalf("verdict = %s, want unknown", report.Verdict)
    }
    if checkStatus(report, "section_conditions") != CheckUnknown {
        t.Fatalf("checks = %+v", report.Checks)
    }
}

func TestEvaluateCompatibilityCompatibleTow(t *testing.T) {
    section := SectionCondition{Section: "section-a", DepthMeters: 2.4, WidthMeters: 6}
    report := EvaluateCompatibility(testPin(), testParams(), section, true, time.Now())
    if report.Verdict != VerdictCompatible {
        t.Fatalf("verdict = %s, checks = %+v", report.Verdict, report.Checks)
    }
    for _, name := range []string{"under_keel_clearance", "depth_to_draft_ratio", "blockage_ratio"} {
        if checkStatus(report, name) != CheckPass {
            t.Fatalf("%s not pass: %+v", name, report.Checks)
        }
    }
}

func TestEvaluateCompatibilityGroundingFails(t *testing.T) {
    params := testParams()
    params.Geometry.DepthMeters = 3
    params.Geometry.DraftMeters = 2.5
    section := SectionCondition{Section: "section-a", DepthMeters: 2.4, WidthMeters: 6}
    report := EvaluateCompatibility(testPin(), params, section, true, time.Now())
    if report.Verdict != VerdictIncompatible {
        t.Fatalf("verdict = %s, want incompatible", report.Verdict)
    }
    if checkStatus(report, "under_keel_clearance") != CheckFail || checkStatus(report, "depth_to_draft_ratio") != CheckFail {
        t.Fatalf("grounding checks not failing: %+v", report.Checks)
    }
}

func TestEvaluateCompatibilityShallowWaterWarns(t *testing.T) {
    params := testParams()
    params.Geometry.DraftMeters = 0.5
    params.Geometry.BeamMeters = 0.3
    section := SectionCondition{Section: "section-a", DepthMeters: 1.4, WidthMeters: 6}
    report := EvaluateCompatibility(testPin(), params, section, true, time.Now())
    if report.Verdict != VerdictMarginal {
        t.Fatalf("verdict = %s, want marginal: %+v", report.Verdict, report.Checks)
    }
    if checkStatus(report, "depth_to_draft_ratio") != CheckWarn {
        t.Fatalf("ratio check not warning: %+v", report.Checks)
    }
}

func TestEvaluateCompatibilityBlockage(t *testing.T) {
    params := testParams()
    params.Geometry.BeamMeters = 1
    params.Geometry.DraftMeters = 0.5
    params.Geometry.DepthMeters = 0.6
    section := SectionCondition{Section: "section-a", DepthMeters: 2, WidthMeters: 4}
    report := EvaluateCompatibility(testPin(), params, section, true, time.Now())
    if report.Verdict != VerdictIncompatible || checkStatus(report, "blockage_ratio") != CheckFail {
        t.Fatalf("blockage not failing: %s %+v", report.Verdict, report.Checks)
    }
}

func TestEvaluateCompatibilityUnknownWidth(t *testing.T) {
    section := SectionCondition{Section: "section-a", DepthMeters: 2.4}
    report := EvaluateCompatibility(testPin(), testParams(), section, true, time.Now())
    if checkStatus(report, "blockage_ratio") != CheckUnknown {
        t.Fatalf("blockage check should be unknown: %+v", report.Checks)
    }
    if report.Verdict != VerdictCompatible {
        t.Fatalf("verdict = %s, want compatible", report.Verdict)
    }
}

func TestSectionConditionValidate(t *testing.T) {
    valid := SectionCondition{Section: "section-a", DepthMeters: 2.4}
    if err := valid.Validate(); err != nil {
        t.Fatalf("valid condition rejected: %v", err)
    }
    invalid := []SectionCondition{
        {Section: " ", DepthMeters: 2.4},
        {Section: "section-a", DepthMeters: 0},
        {Section: "section-a", DepthMeters: 2.4, WidthMeters: -1},
    }
    for index, condition := range invalid {
        if err := condition.Validate(); err == nil {
            t.Fatalf("case %d accepted invalid condition", index)
        }
    }
    if err := valid.Validate(); err != nil || !strings.EqualFold(valid.Section, "section-a") {
        t.Fatal("valid condition changed")
    }
}
