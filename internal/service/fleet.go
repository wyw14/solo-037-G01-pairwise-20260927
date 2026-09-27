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

// commitJSONFile durably replaces path with the encoded value using a
// temporary file and rename, the same commit-before-publish pattern as the
// measurement archive. An empty path means in-memory only.
func commitJSONFile(path, tempPattern string, value any) error {
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), tempPattern)
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
	return os.Rename(name, path)
}

// FleetArchive is the durable geometry and load archive of ship models. Every
// configuration change is appended as a new immutable version so runs keep
// referring to the version they were created with.
type FleetArchive struct {
	mu     sync.Mutex
	path   string
	models map[string]model.ShipModelProfile
}

func OpenFleetArchive(directory string) (*FleetArchive, error) {
	a := &FleetArchive{models: make(map[string]model.ShipModelProfile)}
	if directory == "" {
		return a, nil
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	a.path = filepath.Join(directory, "model-profiles.json")
	data, err := os.ReadFile(a.path)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &a.models); err != nil {
		return nil, fmt.Errorf("read fleet archive: %w", err)
	}
	if a.models == nil {
		return nil, errors.New("fleet archive must contain a model map")
	}
	return a, nil
}

func cloneProfile(value model.ShipModelProfile) model.ShipModelProfile {
	value.Versions = append([]model.ProfileVersion(nil), value.Versions...)
	return value
}

// RecordVersion appends a new configuration version for a model, creating the
// model entry when it is first registered.
func (a *FleetArchive) RecordVersion(name string, params model.ProfileParams, note string) (model.ProfileVersion, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.ProfileVersion{}, errors.New("model name is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	profile := a.models[name]
	profile.Name = name
	updated, version, err := profile.Append(params, note, time.Now().UTC())
	if err != nil {
		return model.ProfileVersion{}, err
	}
	next := make(map[string]model.ShipModelProfile, len(a.models)+1)
	for key, value := range a.models {
		next[key] = value
	}
	next[name] = updated
	if err := commitJSONFile(a.path, "fleet-*.tmp", next); err != nil {
		return model.ProfileVersion{}, err
	}
	a.models = next
	return version, nil
}

func (a *FleetArchive) Get(name string) (model.ShipModelProfile, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	profile, exists := a.models[strings.TrimSpace(name)]
	return cloneProfile(profile), exists
}

func (a *FleetArchive) Current(name string) (model.ProfileVersion, bool) {
	profile, exists := a.Get(name)
	if !exists {
		return model.ProfileVersion{}, false
	}
	return profile.Current()
}

func (a *FleetArchive) List() []model.ShipModelProfile {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]model.ShipModelProfile, 0, len(a.models))
	for _, profile := range a.models {
		result = append(result, cloneProfile(profile))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (r *Runtime) RecordModelVersion(name string, params model.ProfileParams, note string) (model.ProfileVersion, error) {
	return r.Fleet.RecordVersion(name, params, note)
}

func (r *Runtime) ModelProfiles() []model.ShipModelProfile { return r.Fleet.List() }

func (r *Runtime) ModelProfile(name string) (model.ShipModelProfile, bool) {
	return r.Fleet.Get(name)
}

// EvaluateRunCompatibility judges the profile version pinned on a run against
// the current water conditions of the run's section. The pinned version is
// resolved from the version history, so reconfiguring the model afterwards
// does not change what an earlier run is evaluated against.
func (r *Runtime) EvaluateRunCompatibility(id model.RunID) (model.CompatibilityReport, error) {
	run, exists := r.Measurements.Get(id)
	if !exists {
		return model.CompatibilityReport{}, errors.New("run not found")
	}
	if run.Profile == nil {
		return model.CompatibilityReport{}, errors.New("run has no pinned model profile")
	}
	profile, exists := r.Fleet.Get(run.Profile.ModelName)
	if !exists {
		return model.CompatibilityReport{}, errors.New("pinned model profile not found")
	}
	version, exists := profile.Version(run.Profile.Revision)
	if !exists {
		return model.CompatibilityReport{}, errors.New("pinned profile version not found")
	}
	condition, sectionKnown := r.Sections.Get(run.Operation.Section)
	if !sectionKnown {
		condition = model.SectionCondition{Section: run.Operation.Section}
	}
	return model.EvaluateCompatibility(*run.Profile, version.Params, condition, sectionKnown, time.Now().UTC()), nil
}
