package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/solo-037/internal/model"
)

type FleetService interface {
	RecordModelVersion(name string, params model.ProfileParams, note string) (model.ProfileVersion, error)
	ModelProfiles() []model.ShipModelProfile
	ModelProfile(name string) (model.ShipModelProfile, bool)
	RecordSectionCondition(model.SectionCondition) (model.SectionCondition, error)
	SectionConditions() []model.SectionCondition
	EvaluateRunCompatibility(model.RunID) (model.CompatibilityReport, error)
}

type recordProfileRequest struct {
	ModelName string              `json:"model_name"`
	Params    model.ProfileParams `json:"params"`
	Note      string              `json:"note"`
}

func (r *Router) modelProfiles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, r.dependencies.ModelProfiles())
}

func (r *Router) recordModelProfile(w http.ResponseWriter, request *http.Request) {
	var input recordProfileRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	version, err := r.dependencies.RecordModelVersion(input.ModelName, input.Params, input.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, version)
}

func (r *Router) modelProfile(w http.ResponseWriter, request *http.Request) {
	profile, ok := r.dependencies.ModelProfile(chi.URLParam(request, "name"))
	if !ok {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}
	writeJSON(w, profile)
}

func (r *Router) sectionConditions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, r.dependencies.SectionConditions())
}

func (r *Router) recordSectionCondition(w http.ResponseWriter, request *http.Request) {
	var input model.SectionCondition
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	condition, err := r.dependencies.RecordSectionCondition(input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, condition)
}

func (r *Router) runCompatibility(w http.ResponseWriter, request *http.Request) {
	id := model.RunID(chi.URLParam(request, "id"))
	if _, ok := r.dependencies.MeasuredRun(id); !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	report, err := r.dependencies.EvaluateRunCompatibility(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, report)
}
