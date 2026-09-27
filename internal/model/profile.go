package model

import (
    "errors"
    "fmt"
    "math"
    "strings"
    "time"
)

// ProfileGeometry records the principal dimensions and draft of a ship model.
type ProfileGeometry struct {
    LengthMeters float64 `json:"length_meters"`
    BeamMeters   float64 `json:"beam_meters"`
    DepthMeters  float64 `json:"depth_meters"`
    DraftMeters  float64 `json:"draft_meters"`
}

// ProfileLoad records the displacement and loading condition of a ship model.
type ProfileLoad struct {
    DisplacementKilograms float64 `json:"displacement_kilograms"`
    LoadCondition         string  `json:"load_condition"`
}

// ProfileInstallation records how the model is mounted for a tow.
type ProfileInstallation struct {
    TowPoint   string   `json:"tow_point"`
    MountType  string   `json:"mount_type"`
    Appendages []string `json:"appendages"`
}

// ProfileParams is one immutable geometry and load configuration of a ship model.
type ProfileParams struct {
    Geometry     ProfileGeometry     `json:"geometry"`
    Load         ProfileLoad         `json:"load"`
    Installation ProfileInstallation `json:"installation"`
}

func finite(value float64) bool {
    return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (p ProfileParams) Validate() error {
    geometry := p.Geometry
    dimensions := []struct {
        name  string
        value float64
    }{
        {"length", geometry.LengthMeters},
        {"beam", geometry.BeamMeters},
        {"depth", geometry.DepthMeters},
        {"draft", geometry.DraftMeters},
    }
    for _, dimension := range dimensions {
        if !finite(dimension.value) || dimension.value <= 0 {
            return fmt.Errorf("%s must be a finite positive value", dimension.name)
        }
    }
    if geometry.DraftMeters > geometry.DepthMeters {
        return errors.New("draft must not exceed moulded depth")
    }
    if !finite(p.Load.DisplacementKilograms) || p.Load.DisplacementKilograms < 0 {
        return errors.New("displacement must be a finite non-negative value")
    }
    return nil
}

// ProfileVersion is an immutable revision of a model configuration. Once
// recorded it is never modified, so runs keep referring to the exact
// configuration they used even after the model is reconfigured.
type ProfileVersion struct {
    Revision   RevisionID    `json:"revision"`
    Version    int           `json:"version"`
    Params     ProfileParams `json:"params"`
    Note       string        `json:"note"`
    RecordedAt time.Time     `json:"recorded_at"`
}

// ShipModelProfile is the geometry and load archive of one ship model.
type ShipModelProfile struct {
    Name     string           `json:"name"`
    Versions []ProfileVersion `json:"versions"`
}

func (p ShipModelProfile) Current() (ProfileVersion, bool) {
    if len(p.Versions) == 0 {
        return ProfileVersion{}, false
    }
    return p.Versions[len(p.Versions)-1], true
}

func (p ShipModelProfile) Version(revision RevisionID) (ProfileVersion, bool) {
    for _, version := range p.Versions {
        if version.Revision == revision {
            return version, true
        }
    }
    return ProfileVersion{}, false
}

func (p ShipModelProfile) Append(params ProfileParams, note string, at time.Time) (ShipModelProfile, ProfileVersion, error) {
    if err := params.Validate(); err != nil {
        return p, ProfileVersion{}, err
    }
    version := ProfileVersion{
        Revision:   NewRevisionID(),
        Version:    len(p.Versions) + 1,
        Params:     params,
        Note:       strings.TrimSpace(note),
        RecordedAt: at.UTC(),
    }
    versions := append(append([]ProfileVersion(nil), p.Versions...), version)
    p.Versions = versions
    return p, version, nil
}

// SectionCondition is the operator-recorded water condition of a tank section.
// A zero WidthMeters means the width has not been recorded.
type SectionCondition struct {
    Section     string     `json:"section"`
    DepthMeters float64    `json:"depth_meters"`
    WidthMeters float64    `json:"width_meters"`
    Revision    RevisionID `json:"revision"`
    RecordedAt  time.Time  `json:"recorded_at"`
}

func (c SectionCondition) Validate() error {
    if strings.TrimSpace(c.Section) == "" {
        return errors.New("section is required")
    }
    if !finite(c.DepthMeters) || c.DepthMeters <= 0 {
        return errors.New("depth must be a finite positive value")
    }
    if !finite(c.WidthMeters) || c.WidthMeters < 0 {
        return errors.New("width must be a finite non-negative value")
    }
    return nil
}

// ProfilePin references the exact profile version a run was created with.
type ProfilePin struct {
    ModelName string     `json:"model_name"`
    Revision  RevisionID `json:"revision"`
    Version   int        `json:"version"`
}

type CheckStatus string

const (
    CheckPass    CheckStatus = "pass"
    CheckWarn    CheckStatus = "warn"
    CheckFail    CheckStatus = "fail"
    CheckUnknown CheckStatus = "unknown"
)

type CompatibilityCheck struct {
    Name    string      `json:"name"`
    Status  CheckStatus `json:"status"`
    Limit   string      `json:"limit"`
    Actual  float64     `json:"actual"`
    Message string      `json:"message"`
}

type CompatibilityVerdict string

const (
    VerdictCompatible   CompatibilityVerdict = "compatible"
    VerdictMarginal     CompatibilityVerdict = "marginal"
    VerdictIncompatible CompatibilityVerdict = "incompatible"
    VerdictUnknown      CompatibilityVerdict = "unknown"
)

// Compatibility limits for judging whether a tow fits the section conditions.
const (
    MinUnderKeelClearanceMeters = 0.5
    MinDepthToDraftRatio        = 3.0
    WarnBlockageRatio           = 0.02
    MaxBlockageRatio            = 0.05
)

type CompatibilityReport struct {
    ModelName   string               `json:"model_name"`
    Pin         ProfilePin           `json:"pin"`
    Section     string               `json:"section"`
    Verdict     CompatibilityVerdict `json:"verdict"`
    Checks      []CompatibilityCheck `json:"checks"`
    EvaluatedAt time.Time            `json:"evaluated_at"`
}

// EvaluateCompatibility judges one pinned configuration against the current
// water conditions of a section. It is advisory only and does not grant a
// safety permit or block any run.
func EvaluateCompatibility(pin ProfilePin, params ProfileParams, section SectionCondition, sectionKnown bool, at time.Time) CompatibilityReport {
    report := CompatibilityReport{
        ModelName:   pin.ModelName,
        Pin:         pin,
        Section:     section.Section,
        Verdict:     VerdictCompatible,
        Checks:      []CompatibilityCheck{},
        EvaluatedAt: at.UTC(),
    }
    if !sectionKnown {
        report.Verdict = VerdictUnknown
        report.Checks = append(report.Checks, CompatibilityCheck{
            Name:    "section_conditions",
            Status:  CheckUnknown,
            Message: "no water conditions recorded for section",
        })
        return report
    }

    clearance := section.DepthMeters - params.Geometry.DraftMeters
    clearanceCheck := CompatibilityCheck{
        Name:   "under_keel_clearance",
        Limit:  fmt.Sprintf(">= %.2f m", MinUnderKeelClearanceMeters),
        Actual: clearance,
    }
    switch {
    case clearance <= 0:
        clearanceCheck.Status = CheckFail
        clearanceCheck.Message = "draft reaches or exceeds water depth"
    case clearance < MinUnderKeelClearanceMeters:
        clearanceCheck.Status = CheckWarn
        clearanceCheck.Message = "under-keel clearance below recommended margin"
    default:
        clearanceCheck.Status = CheckPass
    }
    report.Checks = append(report.Checks, clearanceCheck)

    ratio := section.DepthMeters / params.Geometry.DraftMeters
    ratioCheck := CompatibilityCheck{
        Name:   "depth_to_draft_ratio",
        Limit:  fmt.Sprintf(">= %.1f", MinDepthToDraftRatio),
        Actual: ratio,
    }
    switch {
    case ratio <= 1:
        ratioCheck.Status = CheckFail
        ratioCheck.Message = "model draft meets or exceeds water depth"
    case ratio < MinDepthToDraftRatio:
        ratioCheck.Status = CheckWarn
        ratioCheck.Message = "shallow-water effects likely at this ratio"
    default:
        ratioCheck.Status = CheckPass
    }
    report.Checks = append(report.Checks, ratioCheck)

    if section.WidthMeters > 0 {
        blockage := params.Geometry.BeamMeters * params.Geometry.DraftMeters / (section.WidthMeters * section.DepthMeters)
        blockageCheck := CompatibilityCheck{
            Name:   "blockage_ratio",
            Limit:  fmt.Sprintf("<= %.2f", MaxBlockageRatio),
            Actual: blockage,
        }
        switch {
        case blockage > MaxBlockageRatio:
            blockageCheck.Status = CheckFail
            blockageCheck.Message = "model blocks too much of the section cross-section"
        case blockage > WarnBlockageRatio:
            blockageCheck.Status = CheckWarn
            blockageCheck.Message = "blockage may distort measurements"
        default:
            blockageCheck.Status = CheckPass
        }
        report.Checks = append(report.Checks, blockageCheck)
    } else {
        report.Checks = append(report.Checks, CompatibilityCheck{
            Name:    "blockage_ratio",
            Status:  CheckUnknown,
            Message: "section width not recorded",
        })
    }

    for _, check := range report.Checks {
        switch check.Status {
        case CheckFail:
            report.Verdict = VerdictIncompatible
        case CheckWarn:
            if report.Verdict == VerdictCompatible {
                report.Verdict = VerdictMarginal
            }
        }
    }
    return report
}
