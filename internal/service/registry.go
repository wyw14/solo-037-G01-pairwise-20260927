package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wyw14/solo-037/internal/model"
)

// MinUnderkeelClearanceMeters is the least water a tow may leave between
// the model's keel and the tank bottom.
const MinUnderkeelClearanceMeters = 0.5

// Compatibility outcomes. Evaluation is advisory: it never blocks a run.
const (
	CompatibilityCompatible   = "compatible"
	CompatibilityIncompatible = "incompatible"
	CompatibilityUnknown      = "unknown"

	checkPass    = "pass"
	checkFail    = "fail"
	checkUnknown = "unknown"
)

type CompatibilityCheck struct {
	Name   string  `json:"name"`
	Status string  `json:"status"`
	Detail string  `json:"detail"`
	Value  float64 `json:"value,omitempty"`
	Limit  float64 `json:"limit,omitempty"`
}

type CompatibilityReport struct {
	Run           model.RunID          `json:"run"`
	ModelName     string               `json:"model_name"`
	Section       string               `json:"section"`
	Status        string               `json:"status"`
	Checks        []CompatibilityCheck `json:"checks"`
	ConfigVersion *int                 `json:"config_version,omitempty"`
	SectionDepth  *float64             `json:"section_depth_meters,omitempty"`
	Evaluated     time.Time            `json:"evaluated_at"`
}

type registryData struct {
	Models   map[string]model.ShipModel        `json:"models"`
	Sections map[string]model.SectionCondition `json:"sections"`
}

// ModelRegistry is the operator-entered archive of ship model geometry and
// loading, plus the section conditions used to judge a tow. Configuration
// versions are append-only so historical runs keep the version they used.
type ModelRegistry struct {
	mu       sync.Mutex
	path     string
	models   map[string]model.ShipModel
	sections map[string]model.SectionCondition
}

func OpenModelRegistry(directory string) (*ModelRegistry, error) {
	r := &ModelRegistry{
		models:   make(map[string]model.ShipModel),
		sections: make(map[string]model.SectionCondition),
	}
	if directory == "" {
		return r, nil
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	r.path = filepath.Join(directory, "model-registry.json")
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	stored := registryData{}
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("read model registry: %w", err)
	}
	if stored.Models == nil || stored.Sections == nil {
		return nil, errors.New("model registry must contain model and section maps")
	}
	r.models = stored.Models
	r.sections = stored.Sections
	return r, nil
}

func cloneShipModel(value model.ShipModel) model.ShipModel {
	value.Versions = append([]model.ModelConfig(nil), value.Versions...)
	return value
}

func (r *ModelRegistry) commit() error {
	if r.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(registryData{Models: r.models, Sections: r.sections}, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(r.path), "registry-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, r.path)
}

// RegisterModel archives a model with its first configuration version.
func (r *ModelRegistry) RegisterModel(name string, draft, length, beam, displacement float64, installation, note string) (model.ShipModel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.ShipModel{}, errors.New("model name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.models[name]; exists {
		return model.ShipModel{}, fmt.Errorf("model %q already registered", name)
	}
	first, err := model.NewModelConfig(1, draft, length, beam, displacement, installation, note, time.Now().UTC())
	if err != nil {
		return model.ShipModel{}, err
	}
	registered := model.NewShipModel(name, first, time.Now().UTC())
	r.models[name] = registered
	if err := r.commit(); err != nil {
		delete(r.models, name)
		return model.ShipModel{}, err
	}
	return cloneShipModel(registered), nil
}

// AddConfig appends a new configuration version; earlier versions stay
// immutable so runs that pinned them remain reproducible.
func (r *ModelRegistry) AddConfig(name string, draft, length, beam, displacement float64, installation, note string) (model.ModelConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ship, exists := r.models[name]
	if !exists {
		return model.ModelConfig{}, fmt.Errorf("model %q not registered", name)
	}
	next, err := model.NewModelConfig(len(ship.Versions)+1, draft, length, beam, displacement, installation, note, time.Now().UTC())
	if err != nil {
		return model.ModelConfig{}, err
	}
	r.models[name] = ship.Append(next)
	if err := r.commit(); err != nil {
		r.models[name] = ship
		return model.ModelConfig{}, err
	}
	return next, nil
}

// CurrentConfig returns the latest configuration for a model, used to pin
// new runs.
func (r *ModelRegistry) CurrentConfig(name string) (model.ModelConfig, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ship, exists := r.models[name]
	if !exists {
		return model.ModelConfig{}, false
	}
	return ship.Current()
}

func (r *ModelRegistry) Model(name string) (model.ShipModel, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ship, exists := r.models[name]
	return cloneShipModel(ship), exists
}

func (r *ModelRegistry) Models() []model.ShipModel {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]model.ShipModel, 0, len(r.models))
	for _, ship := range r.models {
		result = append(result, cloneShipModel(ship))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// SetSectionCondition records the operator-entered water depth for a
// section, replacing any previous entry.
func (r *ModelRegistry) SetSectionCondition(section string, depth float64, note string) (model.SectionCondition, error) {
	section = strings.TrimSpace(section)
	if section == "" {
		return model.SectionCondition{}, errors.New("section is required")
	}
	condition, err := model.NewSectionCondition(section, depth, note, time.Now().UTC())
	if err != nil {
		return model.SectionCondition{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	previous, existed := r.sections[section]
	r.sections[section] = condition
	if err := r.commit(); err != nil {
		if existed {
			r.sections[section] = previous
		} else {
			delete(r.sections, section)
		}
		return model.SectionCondition{}, err
	}
	return condition, nil
}

func (r *ModelRegistry) SectionConditions() []model.SectionCondition {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]model.SectionCondition, 0, len(r.sections))
	for _, condition := range r.sections {
		result = append(result, condition)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Section < result[j].Section })
	return result
}

// Evaluate judges a run's pinned configuration against the current section
// condition. The only hard check is underkeel clearance; missing inputs
// downgrade the report to unknown rather than failing the tow.
func (r *ModelRegistry) Evaluate(run MeasuredRun) CompatibilityReport {
	report := CompatibilityReport{
		Run:       run.Operation.ID,
		ModelName: run.Operation.ModelName,
		Section:   run.Operation.Section,
		Checks:    []CompatibilityCheck{},
		Evaluated: time.Now().UTC(),
	}
	var draft float64
	if run.ModelConfig == nil {
		report.Checks = append(report.Checks, CompatibilityCheck{
			Name:   "model_config",
			Status: checkUnknown,
			Detail: "no configuration version pinned to this run",
		})
	} else {
		draft = run.ModelConfig.DraftMeters
		version := run.ModelConfig.Version
		report.ConfigVersion = &version
		report.Checks = append(report.Checks, CompatibilityCheck{
			Name:   "model_config",
			Status: checkPass,
			Detail: fmt.Sprintf("configuration version %d pinned", version),
		})
	}
	r.mu.Lock()
	condition, recorded := r.sections[run.Operation.Section]
	r.mu.Unlock()
	var depth float64
	if !recorded {
		report.Checks = append(report.Checks, CompatibilityCheck{
			Name:   "section_condition",
			Status: checkUnknown,
			Detail: "section condition not recorded",
		})
	} else {
		depth = condition.DepthMeters
		report.SectionDepth = &depth
		report.Checks = append(report.Checks, CompatibilityCheck{
			Name:   "section_condition",
			Status: checkPass,
			Detail: fmt.Sprintf("section depth %.3f m recorded", depth),
		})
	}
	if run.ModelConfig != nil && recorded {
		clearance := depth - draft
		check := CompatibilityCheck{
			Name:   "underkeel_clearance",
			Value:  clearance,
			Limit:  MinUnderkeelClearanceMeters,
			Status: checkPass,
			Detail: fmt.Sprintf("underkeel clearance %.3f m meets minimum %.3f m", clearance, MinUnderkeelClearanceMeters),
		}
		if clearance < MinUnderkeelClearanceMeters {
			check.Status = checkFail
			check.Detail = fmt.Sprintf("underkeel clearance %.3f m below minimum %.3f m", clearance, MinUnderkeelClearanceMeters)
		}
		report.Checks = append(report.Checks, check)
	}
	report.Status = CompatibilityCompatible
	for _, check := range report.Checks {
		if check.Status == checkFail {
			report.Status = CompatibilityIncompatible
			break
		}
		if check.Status == checkUnknown {
			report.Status = CompatibilityUnknown
		}
	}
	return report
}

func (r *Runtime) RegisterShipModel(name string, draft, length, beam, displacement float64, installation, note string) (model.ShipModel, error) {
	return r.Registry.RegisterModel(name, draft, length, beam, displacement, installation, note)
}

func (r *Runtime) AddShipModelConfig(name string, draft, length, beam, displacement float64, installation, note string) (model.ModelConfig, error) {
	return r.Registry.AddConfig(name, draft, length, beam, displacement, installation, note)
}

func (r *Runtime) ShipModels() []model.ShipModel { return r.Registry.Models() }

func (r *Runtime) ShipModel(name string) (model.ShipModel, bool) { return r.Registry.Model(name) }

func (r *Runtime) SetSectionCondition(section string, depth float64, note string) (model.SectionCondition, error) {
	return r.Registry.SetSectionCondition(section, depth, note)
}

func (r *Runtime) SectionConditions() []model.SectionCondition { return r.Registry.SectionConditions() }

func (r *Runtime) EvaluateRunCompatibility(id model.RunID) (CompatibilityReport, error) {
	run, exists := r.Measurements.Get(id)
	if !exists {
		return CompatibilityReport{}, errors.New("run not found")
	}
	return r.Registry.Evaluate(run), nil
}
