package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wyw14/solo-037/internal/service"
)

func TestShipModelRegistryHTTPFlow(t *testing.T) {
	runtime, err := service.OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewRouter(runtime).Handler()

	registered := httptest.NewRecorder()
	server.ServeHTTP(registered, httptest.NewRequest(http.MethodPost, "/api/ship-models", strings.NewReader(`{"name":"hull-a","draft_meters":0.4,"length_meters":3.2,"beam_meters":0.5,"displacement_kilograms":120,"installation":"bare hull"}`)))
	if registered.Code != http.StatusOK {
		t.Fatalf("register: %d %s", registered.Code, registered.Body)
	}

	condition := httptest.NewRecorder()
	server.ServeHTTP(condition, httptest.NewRequest(http.MethodPut, "/api/section-conditions/section-a", strings.NewReader(`{"depth_meters":1.0}`)))
	if condition.Code != http.StatusOK {
		t.Fatalf("condition: %d %s", condition.Code, condition.Body)
	}

	created := httptest.NewRecorder()
	server.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/measured-runs", strings.NewReader(`{"name":"tow A","model_name":"hull-a","section":"section-a","carriage":"carriage-1"}`)))
	if created.Code != http.StatusOK {
		t.Fatalf("create run: %d %s", created.Code, created.Body)
	}
	var runResponse struct {
		Data service.MeasuredRun `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &runResponse); err != nil {
		t.Fatal(err)
	}
	if runResponse.Data.ModelConfig == nil || runResponse.Data.ModelConfig.Version != 1 || runResponse.Data.ModelConfig.DraftMeters != 0.4 {
		t.Fatalf("run did not pin config version 1: %+v", runResponse.Data.ModelConfig)
	}
	id := string(runResponse.Data.Operation.ID)

	compatibility := httptest.NewRecorder()
	server.ServeHTTP(compatibility, httptest.NewRequest(http.MethodGet, "/api/measured-runs/"+id+"/compatibility", nil))
	if compatibility.Code != http.StatusOK || !strings.Contains(compatibility.Body.String(), `"status":"compatible"`) {
		t.Fatalf("compatibility: %d %s", compatibility.Code, compatibility.Body)
	}

	reconfigured := httptest.NewRecorder()
	server.ServeHTTP(reconfigured, httptest.NewRequest(http.MethodPost, "/api/ship-models/hull-a/configs", strings.NewReader(`{"draft_meters":0.6,"length_meters":3.2,"beam_meters":0.5,"displacement_kilograms":150,"installation":"loaded","note":"deeper ballast"}`)))
	if reconfigured.Code != http.StatusOK {
		t.Fatalf("reconfigure: %d %s", reconfigured.Code, reconfigured.Body)
	}

	detail := httptest.NewRecorder()
	server.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/api/ship-models/hull-a", nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"version":2`) {
		t.Fatalf("detail: %d %s", detail.Code, detail.Body)
	}

	historical := httptest.NewRecorder()
	server.ServeHTTP(historical, httptest.NewRequest(http.MethodGet, "/api/measured-runs/"+id+"/compatibility", nil))
	if historical.Code != http.StatusOK || !strings.Contains(historical.Body.String(), `"config_version":1`) || !strings.Contains(historical.Body.String(), `"status":"compatible"`) {
		t.Fatalf("historical run must keep version 1: %d %s", historical.Code, historical.Body)
	}
}

func TestShipModelHTTPRejectsUnknownAndInvalid(t *testing.T) {
	runtime, err := service.OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewRouter(runtime).Handler()

	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/ship-models/ghost", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown model: %d", missing.Code)
	}
	configOnMissing := httptest.NewRecorder()
	server.ServeHTTP(configOnMissing, httptest.NewRequest(http.MethodPost, "/api/ship-models/ghost/configs", strings.NewReader(`{"draft_meters":0.1,"length_meters":1,"beam_meters":0.2}`)))
	if configOnMissing.Code != http.StatusNotFound {
		t.Fatalf("config on unknown model: %d", configOnMissing.Code)
	}
	invalid := httptest.NewRecorder()
	server.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/ship-models", strings.NewReader(`{"name":"hull-b","draft_meters":-1,"length_meters":3.2,"beam_meters":0.5}`)))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid geometry: %d", invalid.Code)
	}
	compatibilityMissing := httptest.NewRecorder()
	server.ServeHTTP(compatibilityMissing, httptest.NewRequest(http.MethodGet, "/api/measured-runs/ghost/compatibility", nil))
	if compatibilityMissing.Code != http.StatusNotFound {
		t.Fatalf("compatibility for unknown run: %d", compatibilityMissing.Code)
	}
}
