package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
)

func setupTestServer(t *testing.T) (*Handlers, *State) {
	t.Helper()
	tmpl, err := LoadTemplates("templates")
	if err != nil {
		t.Fatalf("loading templates: %v", err)
	}
	state := NewState()
	h := NewHandlers(state, tmpl)
	return h, state
}

func TestGetIndex_Empty(t *testing.T) {
	h, _ := setupTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.handleIndex(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Dashboard") {
		t.Error("expected Dashboard in body")
	}
	if !strings.Contains(body, "No tests have been run yet") {
		t.Error("expected empty state message")
	}
}

func TestGetConfigure_Form(t *testing.T) {
	h, _ := setupTestServer(t)
	req := httptest.NewRequest("GET", "/configure", nil)
	w := httptest.NewRecorder()
	h.handleConfigure(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Configure Test") {
		t.Error("expected form title in body")
	}
	if !strings.Contains(body, "Run Test") {
		t.Error("expected Run Test button")
	}
}

func TestPostConfigure_AddEndpoint(t *testing.T) {
	h, _ := setupTestServer(t)
	vals := url.Values{
		"action":              {"add_endpoint"},
		"mode":                {"vu"},
		"load_style":          {"shorthand"},
		"endpoints[0].method": {"GET"},
		"endpoints[0].url":    {"http://localhost/a"},
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Should show "Endpoint 2" since we started with one and added one
	if !strings.Contains(body, "Endpoint 2") {
		t.Error("expected Endpoint 2 after adding")
	}
}

func TestPostConfigure_RemoveEndpoint(t *testing.T) {
	h, _ := setupTestServer(t)
	vals := url.Values{
		"action":              {"remove_endpoint_1"},
		"mode":                {"vu"},
		"load_style":          {"shorthand"},
		"endpoints[0].method": {"GET"},
		"endpoints[0].url":    {"http://localhost/a"},
		"endpoints[1].method": {"POST"},
		"endpoints[1].url":    {"http://localhost/b"},
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Should only have Endpoint 1 now
	if strings.Contains(body, "Endpoint 2") {
		t.Error("should not have Endpoint 2 after removal")
	}
}

func TestPostConfigure_AddStage(t *testing.T) {
	h, _ := setupTestServer(t)
	vals := url.Values{
		"action":    {"add_stage"},
		"mode":      {"vu"},
		"load_style": {"stages"},
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPostConfigure_AddVariable(t *testing.T) {
	h, _ := setupTestServer(t)
	vals := url.Values{
		"action":    {"add_variable"},
		"mode":      {"vu"},
		"load_style": {"shorthand"},
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPostConfigure_AddHeader(t *testing.T) {
	h, _ := setupTestServer(t)
	vals := url.Values{
		"action":              {"add_header_0"},
		"mode":                {"vu"},
		"load_style":          {"shorthand"},
		"endpoints[0].method": {"GET"},
		"endpoints[0].url":    {"http://localhost/a"},
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPostConfigure_SwitchLoadStyle(t *testing.T) {
	h, _ := setupTestServer(t)
	vals := url.Values{
		"action":    {"switch_load_style"},
		"mode":      {"vu"},
		"load_style": {"shorthand"},
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Load Stages") {
		t.Error("expected stages view after switching from shorthand")
	}
}

func TestPostConfigure_RunInvalid(t *testing.T) {
	h, _ := setupTestServer(t)
	vals := url.Values{
		"action":              {"run"},
		"mode":                {"vu"},
		"load_style":          {"shorthand"},
		"ramp_up":             {"10s"},
		"steady_state":        {"30s"},
		"ramp_down":           {"10s"},
		"max_vus":             {"10"},
		"endpoints[0].method": {"GET"},
		"endpoints[0].url":    {""},  // empty URL = validation error
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 (re-render with errors), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Please fix the following errors") {
		t.Error("expected error message in body")
	}
}

func TestPostConfigure_RunValid(t *testing.T) {
	// Create a mock target server
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	h, _ := setupTestServer(t)
	vals := url.Values{
		"action":                       {"run"},
		"name":                         {"Test Run"},
		"mode":                         {"vu"},
		"load_style":                   {"shorthand"},
		"ramp_up":                      {"500ms"},
		"steady_state":                 {"500ms"},
		"ramp_down":                    {"500ms"},
		"max_vus":                      {"2"},
		"timeout":                      {"5s"},
		"output_format":                {"console"},
		"output_interval":              {"1s"},
		"endpoints[0].name":            {"health"},
		"endpoints[0].method":          {"GET"},
		"endpoints[0].url":             {target.URL + "/health"},
		"endpoints[0].weight":          {"1"},
		"endpoints[0].expect_status":   {"200"},
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	// Should redirect to test page
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/test/") {
		t.Errorf("expected redirect to /test/{id}, got %q", loc)
	}
}

func TestGetTestStatus_NotFound(t *testing.T) {
	h, _ := setupTestServer(t)
	req := httptest.NewRequest("GET", "/test/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	h.handleTestStatus(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetTestStatus_Running(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	h, state := setupTestServer(t)

	cfg := &config.Config{
		Name: "running-test",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 10 * time.Second}, Target: 2},
			},
		},
		HTTP:    config.HTTPConfig{Timeout: config.Duration{Duration: 5 * time.Second}},
		Output:  config.OutputConfig{Format: "console", Interval: config.Duration{Duration: 5 * time.Second}},
		Endpoints: []config.Endpoint{
			{Name: "health", Method: "GET", URL: target.URL, Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		},
	}

	run := state.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}
	defer state.StopTest(run.ID)

	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)

	req := httptest.NewRequest("GET", "/test/"+run.ID, nil)
	req.SetPathValue("id", run.ID)
	w := httptest.NewRecorder()
	h.handleTestStatus(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "running") {
		t.Error("expected running badge")
	}
	if !strings.Contains(body, "meta http-equiv") {
		t.Error("expected meta refresh tag")
	}
}

func TestGetTestStatus_Completed(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	h, state := setupTestServer(t)

	cfg := &config.Config{
		Name: "quick-test",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 200 * time.Millisecond}, Target: 1},
			},
		},
		HTTP:    config.HTTPConfig{Timeout: config.Duration{Duration: 5 * time.Second}},
		Output:  config.OutputConfig{Format: "console", Interval: config.Duration{Duration: 10 * time.Second}},
		Endpoints: []config.Endpoint{
			{Name: "health", Method: "GET", URL: target.URL, Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		},
	}

	run := state.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start test")
	}

	// Wait for it to complete
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if run.Status != "running" {
			break
		}
	}
	if run.Status == "running" {
		t.Fatal("test did not complete in time")
	}

	req := httptest.NewRequest("GET", "/test/"+run.ID, nil)
	req.SetPathValue("id", run.ID)
	w := httptest.NewRecorder()
	h.handleTestStatus(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "meta http-equiv") {
		t.Error("should not have meta refresh for completed test")
	}
	if !strings.Contains(body, "Total Requests") {
		t.Error("expected results content")
	}
}

func TestPostConfigure_RunWhileAlreadyRunning(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	h, state := setupTestServer(t)

	cfg := &config.Config{
		Name: "long-test",
		Load: config.LoadConfig{
			Mode: "vu",
			Stages: []config.Stage{
				{Duration: config.Duration{Duration: 30 * time.Second}, Target: 1},
			},
		},
		HTTP:    config.HTTPConfig{Timeout: config.Duration{Duration: 5 * time.Second}},
		Output:  config.OutputConfig{Format: "console", Interval: config.Duration{Duration: 5 * time.Second}},
		Endpoints: []config.Endpoint{
			{Name: "health", Method: "GET", URL: target.URL, Weight: 1, Expect: config.ExpectConfig{Status: 200}},
		},
	}

	run := state.StartTest(cfg)
	if run == nil {
		t.Fatal("failed to start first test")
	}
	defer state.StopTest(run.ID)

	vals := url.Values{
		"action":                       {"run"},
		"mode":                         {"vu"},
		"load_style":                   {"shorthand"},
		"ramp_up":                      {"1s"},
		"steady_state":                 {"1s"},
		"ramp_down":                    {"1s"},
		"max_vus":                      {"1"},
		"timeout":                      {"5s"},
		"output_format":                {"console"},
		"output_interval":              {"1s"},
		"endpoints[0].method":          {"GET"},
		"endpoints[0].url":             {target.URL},
		"endpoints[0].weight":          {"1"},
		"endpoints[0].expect_status":   {"200"},
	}
	req := httptest.NewRequest("POST", "/configure", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleConfigurePost(w, req)

	// Should re-render with error, not redirect
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "already running") {
		t.Error("expected 'already running' error message")
	}
}
