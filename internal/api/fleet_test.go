package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wyw14/solo-037/internal/service"
)

func profileBody(draft float64) string {
	return `{"model_name":"hull-a","params":{` +
		`"geometry":{"length_meters":4,"beam_meters":0.6,"depth_meters":0.35,"draft_meters":` + jsonNumber(draft) + `},` +
		`"load":{"displacement_kilograms":180,"load_condition":"full_load"},` +
		`"installation":{"tow_point":"bow","mount_type":"strut","appendages":["rudder"]}` +
		`},"note":"initial"}`
}

func jsonNumber(value float64) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func TestModelProfileAndCompatibilityHTTP(t *testing.T) {
	runtime, err := service.OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewRouter(runtime).Handler()

	created := httptest.NewRecorder()
	server.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/model-profiles", strings.NewReader(profileBody(0.2))))
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"version":1`) {
		t.Fatalf("record profile: %d %s", created.Code, created.Body)
	}

	condition := httptest.NewRecorder()
	server.ServeHTTP(condition, httptest.NewRequest(http.MethodPost, "/api/section-conditions", strings.NewReader(`{"section":"section-a","depth_meters":2.4,"width_meters":6}`)))
	if condition.Code != http.StatusOK {
		t.Fatalf("record condition: %d %s", condition.Code, condition.Body)
	}

	run := httptest.NewRecorder()
	server.ServeHTTP(run, httptest.NewRequest(http.MethodPost, "/api/measured-runs", strings.NewReader(`{"name":"tow A","model_name":"hull-a","section":"section-a","carriage":"carriage-1"}`)))
	if run.Code != http.StatusOK {
		t.Fatalf("create run: %d %s", run.Code, run.Body)
	}
	var runResponse struct {
		Data service.MeasuredRun `json:"data"`
	}
	if err := json.Unmarshal(run.Body.Bytes(), &runResponse); err != nil {
		t.Fatal(err)
	}
	if runResponse.Data.Profile == nil || runResponse.Data.Profile.Version != 1 {
		t.Fatalf("run not pinned: %+v", runResponse.Data.Profile)
	}

	compatibility := httptest.NewRecorder()
	server.ServeHTTP(compatibility, httptest.NewRequest(http.MethodGet, "/api/measured-runs/"+string(runResponse.Data.Operation.ID)+"/compatibility", nil))
	if compatibility.Code != http.StatusOK || !strings.Contains(compatibility.Body.String(), `"verdict":"compatible"`) {
		t.Fatalf("compatibility: %d %s", compatibility.Code, compatibility.Body)
	}

	profile := httptest.NewRecorder()
	server.ServeHTTP(profile, httptest.NewRequest(http.MethodGet, "/api/model-profiles/hull-a", nil))
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), `"draft_meters":0.2`) {
		t.Fatalf("get profile: %d %s", profile.Code, profile.Body)
	}

	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/model-profiles/unknown", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing model: %d", missing.Code)
	}
}

func TestCompatibilityHTTPRejectsUnknownAndUnpinnedRuns(t *testing.T) {
	runtime, err := service.OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewRouter(runtime).Handler()

	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/measured-runs/missing/compatibility", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing run: %d", missing.Code)
	}

	run := httptest.NewRecorder()
	server.ServeHTTP(run, httptest.NewRequest(http.MethodPost, "/api/measured-runs", strings.NewReader(`{"name":"tow B","model_name":"unregistered","section":"section-a","carriage":"carriage-1"}`)))
	var runResponse struct {
		Data service.MeasuredRun `json:"data"`
	}
	if err := json.Unmarshal(run.Body.Bytes(), &runResponse); err != nil {
		t.Fatal(err)
	}
	unpinned := httptest.NewRecorder()
	server.ServeHTTP(unpinned, httptest.NewRequest(http.MethodGet, "/api/measured-runs/"+string(runResponse.Data.Operation.ID)+"/compatibility", nil))
	if unpinned.Code != http.StatusBadRequest {
		t.Fatalf("unpinned run: %d %s", unpinned.Code, unpinned.Body)
	}

	invalid := httptest.NewRecorder()
	server.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/model-profiles", strings.NewReader(`{"model_name":"hull-b","params":{"geometry":{"length_meters":4,"beam_meters":0.6,"depth_meters":0.35,"draft_meters":0}}}`)))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid profile: %d %s", invalid.Code, invalid.Body)
	}
}
