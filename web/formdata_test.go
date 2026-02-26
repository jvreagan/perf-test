package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDefaultFormData(t *testing.T) {
	fd := DefaultFormData()
	if fd.Mode != "vu" {
		t.Errorf("expected mode=vu, got %q", fd.Mode)
	}
	if fd.LoadStyle != "shorthand" {
		t.Errorf("expected load_style=shorthand, got %q", fd.LoadStyle)
	}
	if len(fd.Endpoints) != 1 {
		t.Errorf("expected 1 default endpoint, got %d", len(fd.Endpoints))
	}
	if fd.Endpoints[0].Method != "GET" {
		t.Errorf("expected default method=GET, got %q", fd.Endpoints[0].Method)
	}
}

func makeFormRequest(values url.Values) *http.Request {
	body := values.Encode()
	req, _ := http.NewRequest("POST", "/configure", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestParseFormData_BasicFields(t *testing.T) {
	vals := url.Values{
		"name":        {"My Test"},
		"description": {"A description"},
		"mode":        {"vu"},
		"load_style":  {"shorthand"},
		"ramp_up":     {"10s"},
		"steady_state":{"60s"},
		"ramp_down":   {"10s"},
		"max_vus":     {"50"},
		"think_time":  {"100ms"},
		"max_rps":     {"200"},
		"timeout":     {"30s"},
		"output_format":   {"json"},
		"output_interval": {"5s"},
		"output_file":     {"results.json"},
	}
	fd := ParseFormData(makeFormRequest(vals))

	if fd.Name != "My Test" {
		t.Errorf("name: got %q", fd.Name)
	}
	if fd.Mode != "vu" {
		t.Errorf("mode: got %q", fd.Mode)
	}
	if fd.RampUp != "10s" {
		t.Errorf("ramp_up: got %q", fd.RampUp)
	}
	if fd.MaxVUs != "50" {
		t.Errorf("max_vus: got %q", fd.MaxVUs)
	}
	if fd.OutputFormat != "json" {
		t.Errorf("output_format: got %q", fd.OutputFormat)
	}
}

func TestParseFormData_Endpoints(t *testing.T) {
	vals := url.Values{
		"endpoints[0].name":          {"Get Users"},
		"endpoints[0].method":        {"GET"},
		"endpoints[0].url":           {"http://localhost/users"},
		"endpoints[0].weight":        {"3"},
		"endpoints[0].expect_status": {"200"},
		"endpoints[0].headers[0].key":   {"Authorization"},
		"endpoints[0].headers[0].value": {"Bearer token123"},
		"endpoints[1].name":          {"Create User"},
		"endpoints[1].method":        {"POST"},
		"endpoints[1].url":           {"http://localhost/users"},
		"endpoints[1].body":          {`{"name":"test"}`},
		"endpoints[1].weight":        {"1"},
		"endpoints[1].expect_status": {"201"},
	}
	fd := ParseFormData(makeFormRequest(vals))

	if len(fd.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(fd.Endpoints))
	}
	if fd.Endpoints[0].Name != "Get Users" {
		t.Errorf("endpoint 0 name: got %q", fd.Endpoints[0].Name)
	}
	if fd.Endpoints[0].URL != "http://localhost/users" {
		t.Errorf("endpoint 0 URL: got %q", fd.Endpoints[0].URL)
	}
	if len(fd.Endpoints[0].Headers) != 1 {
		t.Fatalf("expected 1 header, got %d", len(fd.Endpoints[0].Headers))
	}
	if fd.Endpoints[0].Headers[0].Key != "Authorization" {
		t.Errorf("header key: got %q", fd.Endpoints[0].Headers[0].Key)
	}
	if fd.Endpoints[1].Method != "POST" {
		t.Errorf("endpoint 1 method: got %q", fd.Endpoints[1].Method)
	}
	if fd.Endpoints[1].Body != `{"name":"test"}` {
		t.Errorf("endpoint 1 body: got %q", fd.Endpoints[1].Body)
	}
}

func TestParseFormData_Stages(t *testing.T) {
	vals := url.Values{
		"stages[0].duration": {"30s"},
		"stages[0].target":   {"10"},
		"stages[0].ramp":     {"linear"},
		"stages[1].duration": {"60s"},
		"stages[1].target":   {"50"},
		"stages[1].ramp":     {"step"},
	}
	fd := ParseFormData(makeFormRequest(vals))

	if len(fd.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(fd.Stages))
	}
	if fd.Stages[0].Duration != "30s" {
		t.Errorf("stage 0 duration: got %q", fd.Stages[0].Duration)
	}
	if fd.Stages[1].Ramp != "step" {
		t.Errorf("stage 1 ramp: got %q", fd.Stages[1].Ramp)
	}
}

func TestParseFormData_Variables(t *testing.T) {
	vals := url.Values{
		"variables[0].key":   {"base_url"},
		"variables[0].value": {"http://localhost:8080"},
		"variables[1].key":   {"token"},
		"variables[1].value": {"abc123"},
	}
	fd := ParseFormData(makeFormRequest(vals))

	if len(fd.Variables) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(fd.Variables))
	}
	if fd.Variables[0].Key != "base_url" {
		t.Errorf("var 0 key: got %q", fd.Variables[0].Key)
	}
	if fd.Variables[1].Value != "abc123" {
		t.Errorf("var 1 value: got %q", fd.Variables[1].Value)
	}
}

func TestToConfig_Shorthand_Valid(t *testing.T) {
	fd := &FormData{
		Name:        "Test",
		Mode:        "vu",
		LoadStyle:   "shorthand",
		RampUp:      "10s",
		SteadyState: "30s",
		RampDown:    "10s",
		MaxVUs:      "10",
		Timeout:     "30s",
		OutputFormat:   "console",
		OutputInterval: "5s",
		Endpoints: []EndpointData{
			{Method: "GET", URL: "http://localhost/health", Weight: "1", ExpectStatus: "200"},
		},
	}
	cfg, err := fd.ToConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "Test" {
		t.Errorf("name: got %q", cfg.Name)
	}
	if len(cfg.Load.Stages) != 3 {
		t.Errorf("expected 3 stages from shorthand, got %d", len(cfg.Load.Stages))
	}
	if len(cfg.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(cfg.Endpoints))
	}
}

func TestToConfig_Stages_Valid(t *testing.T) {
	fd := &FormData{
		Mode:      "vu",
		LoadStyle: "stages",
		Timeout:   "30s",
		OutputFormat:   "console",
		OutputInterval: "5s",
		Stages: []StageData{
			{Duration: "30s", Target: "10", Ramp: "linear"},
			{Duration: "60s", Target: "10", Ramp: "linear"},
			{Duration: "30s", Target: "0", Ramp: "linear"},
		},
		Endpoints: []EndpointData{
			{Method: "GET", URL: "http://localhost/health", Weight: "1", ExpectStatus: "200"},
		},
	}
	cfg, err := fd.ToConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Load.Stages) != 3 {
		t.Errorf("expected 3 stages, got %d", len(cfg.Load.Stages))
	}
	if cfg.Load.Stages[0].Target != 10 {
		t.Errorf("stage 0 target: got %d", cfg.Load.Stages[0].Target)
	}
}

func TestToConfig_InvalidDuration(t *testing.T) {
	fd := &FormData{
		Mode:      "vu",
		LoadStyle: "shorthand",
		RampUp:    "not-a-duration",
		MaxVUs:    "10",
		Endpoints: []EndpointData{
			{Method: "GET", URL: "http://localhost/health", Weight: "1", ExpectStatus: "200"},
		},
	}
	_, err := fd.ToConfig()
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
	if !strings.Contains(err.Error(), "ramp up") {
		t.Errorf("error should mention ramp up: %v", err)
	}
}

func TestToConfig_MissingURL(t *testing.T) {
	fd := &FormData{
		Mode:        "vu",
		LoadStyle:   "shorthand",
		RampUp:      "10s",
		SteadyState: "30s",
		RampDown:    "10s",
		MaxVUs:      "10",
		Endpoints: []EndpointData{
			{Method: "GET", URL: "", Weight: "1", ExpectStatus: "200"},
		},
	}
	_, err := fd.ToConfig()
	if err == nil {
		t.Fatal("expected validation error for missing URL")
	}
	if !strings.Contains(err.Error(), "URL is required") {
		t.Errorf("error should mention URL: %v", err)
	}
}

func TestToConfig_ArrivalRateMode(t *testing.T) {
	fd := &FormData{
		Mode:      "arrival_rate",
		LoadStyle: "stages",
		Timeout:   "30s",
		OutputFormat:   "console",
		OutputInterval: "5s",
		Stages: []StageData{
			{Duration: "30s", Target: "50", Ramp: "linear"},
		},
		Endpoints: []EndpointData{
			{Method: "GET", URL: "http://localhost/health", Weight: "1", ExpectStatus: "200"},
		},
	}
	cfg, err := fd.ToConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Load.Mode != "arrival_rate" {
		t.Errorf("mode: got %q", cfg.Load.Mode)
	}
}

func TestToConfig_WithVariablesAndHeaders(t *testing.T) {
	fd := &FormData{
		Mode:        "vu",
		LoadStyle:   "shorthand",
		RampUp:      "10s",
		SteadyState: "30s",
		RampDown:    "10s",
		MaxVUs:      "5",
		Timeout:     "30s",
		OutputFormat:   "console",
		OutputInterval: "5s",
		Variables: []VariableData{
			{Key: "base_url", Value: "http://localhost"},
		},
		Endpoints: []EndpointData{
			{
				Method: "POST", URL: "http://localhost/api", Weight: "1", ExpectStatus: "201",
				Body: `{"name":"test"}`,
				Headers: []HeaderData{
					{Key: "Content-Type", Value: "application/json"},
					{Key: "Authorization", Value: "Bearer tok"},
				},
			},
		},
	}
	cfg, err := fd.ToConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Variables["base_url"] != "http://localhost" {
		t.Errorf("variable base_url: got %q", cfg.Variables["base_url"])
	}
	if cfg.Endpoints[0].Headers["Content-Type"] != "application/json" {
		t.Errorf("header Content-Type: got %q", cfg.Endpoints[0].Headers["Content-Type"])
	}
	if cfg.Endpoints[0].Body != `{"name":"test"}` {
		t.Errorf("body: got %q", cfg.Endpoints[0].Body)
	}
}

func TestToConfig_InvalidMaxRPS(t *testing.T) {
	fd := &FormData{
		Mode:      "vu",
		LoadStyle: "shorthand",
		RampUp:    "10s",
		SteadyState: "30s",
		RampDown:  "10s",
		MaxVUs:    "10",
		MaxRPS:    "not-a-number",
		Endpoints: []EndpointData{
			{Method: "GET", URL: "http://localhost/health", Weight: "1", ExpectStatus: "200"},
		},
	}
	_, err := fd.ToConfig()
	if err == nil {
		t.Fatal("expected error for invalid max RPS")
	}
}

func TestToConfig_InvalidStageTarget(t *testing.T) {
	fd := &FormData{
		Mode:      "vu",
		LoadStyle: "stages",
		Stages: []StageData{
			{Duration: "30s", Target: "abc", Ramp: "linear"},
		},
		Endpoints: []EndpointData{
			{Method: "GET", URL: "http://localhost/health", Weight: "1", ExpectStatus: "200"},
		},
	}
	_, err := fd.ToConfig()
	if err == nil {
		t.Fatal("expected error for invalid stage target")
	}
}

func TestTargetLabel(t *testing.T) {
	if got := TargetLabel("vu"); got != "VUs" {
		t.Errorf("expected VUs, got %q", got)
	}
	if got := TargetLabel("arrival_rate"); got != "RPS" {
		t.Errorf("expected RPS, got %q", got)
	}
}
