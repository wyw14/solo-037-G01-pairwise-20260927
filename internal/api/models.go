package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/solo-037/internal/model"
	"github.com/wyw14/solo-037/internal/service"
)

type ModelService interface {
	RegisterShipModel(name string, draft, length, beam, displacement float64, installation, note string) (model.ShipModel, error)
	AddShipModelConfig(name string, draft, length, beam, displacement float64, installation, note string) (model.ModelConfig, error)
	ShipModels() []model.ShipModel
	ShipModel(name string) (model.ShipModel, bool)
	SetSectionCondition(section string, depth float64, note string) (model.SectionCondition, error)
	SectionConditions() []model.SectionCondition
	EvaluateRunCompatibility(model.RunID) (service.CompatibilityReport, error)
}

type shipModelConfigRequest struct {
	DraftMeters           float64 `json:"draft_meters"`
	LengthMeters          float64 `json:"length_meters"`
	BeamMeters            float64 `json:"beam_meters"`
	DisplacementKilograms float64 `json:"displacement_kilograms"`
	Installation          string  `json:"installation"`
	Note                  string  `json:"note"`
}

type createShipModelRequest struct {
	Name string `json:"name"`
	shipModelConfigRequest
}

type sectionConditionRequest struct {
	DepthMeters float64 `json:"depth_meters"`
	Note        string  `json:"note"`
}

func (r *Router) shipModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, r.dependencies.ShipModels())
}

func (r *Router) createShipModel(w http.ResponseWriter, request *http.Request) {
	var input createShipModelRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	config := input.shipModelConfigRequest
	value, err := r.dependencies.RegisterShipModel(input.Name, config.DraftMeters, config.LengthMeters, config.BeamMeters, config.DisplacementKilograms, config.Installation, config.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, value)
}

func (r *Router) shipModel(w http.ResponseWriter, request *http.Request) {
	value, ok := r.dependencies.ShipModel(chi.URLParam(request, "name"))
	if !ok {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}
	writeJSON(w, value)
}

func (r *Router) addShipModelConfig(w http.ResponseWriter, request *http.Request) {
	name := chi.URLParam(request, "name")
	if _, ok := r.dependencies.ShipModel(name); !ok {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}
	var input shipModelConfigRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := r.dependencies.AddShipModelConfig(name, input.DraftMeters, input.LengthMeters, input.BeamMeters, input.DisplacementKilograms, input.Installation, input.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, value)
}

func (r *Router) sectionConditions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, r.dependencies.SectionConditions())
}

func (r *Router) setSectionCondition(w http.ResponseWriter, request *http.Request) {
	var input sectionConditionRequest
	if err := decodeMeasurementBody(request, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := r.dependencies.SetSectionCondition(chi.URLParam(request, "section"), input.DepthMeters, input.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, value)
}

func (r *Router) runCompatibility(w http.ResponseWriter, request *http.Request) {
	value, err := r.dependencies.EvaluateRunCompatibility(model.RunID(chi.URLParam(request, "id")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, value)
}
