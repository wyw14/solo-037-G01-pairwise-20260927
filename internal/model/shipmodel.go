package model

import (
	"errors"
	"math"
	"time"
)

// ModelConfig is one immutable version of a ship model's geometry and
// loading. Versions are append-only: a measured run pins the version in
// force when it is created, so later configuration changes never rewrite
// what an earlier trial used.
type ModelConfig struct {
	Version               int        `json:"version"`
	Revision              RevisionID `json:"revision"`
	DraftMeters           float64    `json:"draft_meters"`
	LengthMeters          float64    `json:"length_meters"`
	BeamMeters            float64    `json:"beam_meters"`
	DisplacementKilograms float64    `json:"displacement_kilograms"`
	Installation          string     `json:"installation"`
	Note                  string     `json:"note,omitempty"`
	RecordedAt            time.Time  `json:"recorded_at"`
}

func NewModelConfig(version int, draft, length, beam, displacement float64, installation, note string, at time.Time) (ModelConfig, error) {
	if !positiveFinite(draft) || !positiveFinite(length) || !positiveFinite(beam) {
		return ModelConfig{}, errors.New("draft, length and beam must be positive finite values")
	}
	if math.IsNaN(displacement) || math.IsInf(displacement, 0) || displacement < 0 {
		return ModelConfig{}, errors.New("displacement must be a non-negative finite value")
	}
	return ModelConfig{
		Version:               version,
		Revision:              NewRevisionID(),
		DraftMeters:           draft,
		LengthMeters:          length,
		BeamMeters:            beam,
		DisplacementKilograms: displacement,
		Installation:          installation,
		Note:                  note,
		RecordedAt:            at.UTC(),
	}, nil
}

func positiveFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

// ShipModel is the archive entry for one physical model, keyed by the name
// that measured runs record as model_name.
type ShipModel struct {
	Name      string        `json:"name"`
	Versions  []ModelConfig `json:"versions"`
	CreatedAt time.Time     `json:"created_at"`
}

func NewShipModel(name string, first ModelConfig, at time.Time) ShipModel {
	return ShipModel{Name: name, Versions: []ModelConfig{first}, CreatedAt: at.UTC()}
}

// Current returns the latest configuration version.
func (m ShipModel) Current() (ModelConfig, bool) {
	if len(m.Versions) == 0 {
		return ModelConfig{}, false
	}
	return m.Versions[len(m.Versions)-1], true
}

// Append adds the next configuration version.
func (m ShipModel) Append(next ModelConfig) ShipModel {
	m.Versions = append(append([]ModelConfig(nil), m.Versions...), next)
	return m
}

// SectionCondition is the operator-entered state of one tank section used
// for compatibility evaluation.
type SectionCondition struct {
	Section     string    `json:"section"`
	DepthMeters float64   `json:"depth_meters"`
	Note        string    `json:"note,omitempty"`
	RecordedAt  time.Time `json:"recorded_at"`
}

func NewSectionCondition(section string, depth float64, note string, at time.Time) (SectionCondition, error) {
	if !positiveFinite(depth) {
		return SectionCondition{}, errors.New("depth must be a positive finite value")
	}
	return SectionCondition{Section: section, DepthMeters: depth, Note: note, RecordedAt: at.UTC()}, nil
}
