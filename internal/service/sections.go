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

// SectionArchive is the durable record of operator-reported water conditions
// per tank section. Recording a condition for a section replaces the previous
// one; compatibility checks always evaluate against the latest record.
type SectionArchive struct {
	mu         sync.Mutex
	path       string
	conditions map[string]model.SectionCondition
}

func OpenSectionArchive(directory string) (*SectionArchive, error) {
	a := &SectionArchive{conditions: make(map[string]model.SectionCondition)}
	if directory == "" {
		return a, nil
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	a.path = filepath.Join(directory, "section-conditions.json")
	data, err := os.ReadFile(a.path)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &a.conditions); err != nil {
		return nil, fmt.Errorf("read section archive: %w", err)
	}
	if a.conditions == nil {
		return nil, errors.New("section archive must contain a condition map")
	}
	return a, nil
}

func (a *SectionArchive) Record(condition model.SectionCondition) (model.SectionCondition, error) {
	if err := condition.Validate(); err != nil {
		return model.SectionCondition{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	condition.Section = strings.TrimSpace(condition.Section)
	condition.Revision = model.NewRevisionID()
	condition.RecordedAt = time.Now().UTC()
	next := make(map[string]model.SectionCondition, len(a.conditions)+1)
	for key, value := range a.conditions {
		next[key] = value
	}
	next[condition.Section] = condition
	if err := commitJSONFile(a.path, "sections-*.tmp", next); err != nil {
		return model.SectionCondition{}, err
	}
	a.conditions = next
	return condition, nil
}

func (a *SectionArchive) Get(name string) (model.SectionCondition, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	condition, exists := a.conditions[strings.TrimSpace(name)]
	return condition, exists
}

func (a *SectionArchive) List() []model.SectionCondition {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]model.SectionCondition, 0, len(a.conditions))
	for _, condition := range a.conditions {
		result = append(result, condition)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Section < result[j].Section })
	return result
}

func (r *Runtime) RecordSectionCondition(condition model.SectionCondition) (model.SectionCondition, error) {
	return r.Sections.Record(condition)
}

func (r *Runtime) SectionConditions() []model.SectionCondition { return r.Sections.List() }
