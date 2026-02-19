package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestLoad_Valid(t *testing.T) {
	yaml := `
name: "Test"
load:
  stages:
    - duration: 10s
      target: 5
endpoints:
  - name: "health"
    url: "http://localhost/health"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", cfg.Name)
	}
	if len(cfg.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(cfg.Endpoints))
	}
	if cfg.Endpoints[0].Method != "GET" {
		t.Errorf("expected default method GET, got %q", cfg.Endpoints[0].Method)
	}
	if cfg.Endpoints[0].Weight != 1 {
		t.Errorf("expected default weight 1, got %d", cfg.Endpoints[0].Weight)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "{{invalid yaml")
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoad_EnvExpansion(t *testing.T) {
	t.Setenv("TEST_TOKEN", "mytoken123")
	yaml := `
load:
  stages:
    - duration: 5s
      target: 1
variables:
  token: "$TEST_TOKEN"
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Env vars are expanded in the variables section only.
	if cfg.Variables["token"] != "mytoken123" {
		t.Errorf("env expansion in variables failed: got %q", cfg.Variables["token"])
	}
}

func TestLoad_EnvExpansion_TemplateVarsPreserved(t *testing.T) {
	// Ensure ${base_url} style config variables are NOT erased by os.ExpandEnv
	yaml := `
load:
  stages:
    - duration: 5s
      target: 1
variables:
  base_url: "http://localhost:9999"
endpoints:
  - url: "${base_url}/health"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Template token must survive in the URL field
	if cfg.Endpoints[0].URL != "${base_url}/health" {
		t.Errorf("template token was erased: got %q", cfg.Endpoints[0].URL)
	}
}

func TestDurationParsing(t *testing.T) {
	yaml := `
load:
  stages:
    - duration: 2m30s
      target: 10
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 2*time.Minute + 30*time.Second
	if cfg.Load.Stages[0].Duration.Duration != expected {
		t.Errorf("expected %v, got %v", expected, cfg.Load.Stages[0].Duration.Duration)
	}
}

func TestNormalizeStages(t *testing.T) {
	yaml := `
load:
  ramp_up: 10s
  steady_state: 30s
  ramp_down: 10s
  max_vus: 50
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Load.Stages) != 3 {
		t.Fatalf("expected 3 stages from shorthand, got %d", len(cfg.Load.Stages))
	}
	if cfg.Load.Stages[0].Target != 50 {
		t.Errorf("ramp_up target: expected 50, got %d", cfg.Load.Stages[0].Target)
	}
	if cfg.Load.Stages[1].Target != 50 {
		t.Errorf("steady_state target: expected 50, got %d", cfg.Load.Stages[1].Target)
	}
	if cfg.Load.Stages[2].Target != 0 {
		t.Errorf("ramp_down target: expected 0, got %d", cfg.Load.Stages[2].Target)
	}
}

func TestValidate_NoEndpoints(t *testing.T) {
	yaml := `
load:
  stages:
    - duration: 5s
      target: 1
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error for no endpoints")
	}
}

func TestValidate_EmptyURL(t *testing.T) {
	yaml := `
load:
  stages:
    - duration: 5s
      target: 1
endpoints:
  - name: "test"
    url: ""
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error for empty URL")
	}
}

func TestValidate_InvalidFormat(t *testing.T) {
	cfg := &Config{
		Endpoints: []Endpoint{{URL: "http://x", Weight: 1, Method: "GET", Expect: ExpectConfig{Status: 200}}},
		Load: LoadConfig{
			Mode:   "vu",
			Stages: []Stage{{Duration: Duration{5 * time.Second}, Target: 1}},
		},
		Output: OutputConfig{Format: "xml", Interval: Duration{5 * time.Second}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid format")
	}
}

func TestValidate_StepRamp_Valid(t *testing.T) {
	yaml := `
name: "step"
load:
  stages:
    - duration: 10s
      target: 50
      ramp: step
    - duration: 30s
      target: 50
    - duration: 10s
      target: 0
      ramp: linear
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Load.Stages[0].Ramp != "step" {
		t.Errorf("expected ramp=step, got %q", cfg.Load.Stages[0].Ramp)
	}
}

func TestValidate_StepRamp_Invalid(t *testing.T) {
	yaml := `
load:
  stages:
    - duration: 10s
      target: 50
      ramp: banana
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error for invalid ramp value")
	}
}

func TestValidate_Mode_ArrivalRate_Valid(t *testing.T) {
	yaml := `
load:
  mode: arrival_rate
  stages:
    - duration: 10s
      target: 100
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Load.Mode != "arrival_rate" {
		t.Errorf("expected mode=arrival_rate, got %q", cfg.Load.Mode)
	}
}

func TestValidate_Mode_Invalid(t *testing.T) {
	yaml := `
load:
  mode: turbo
  stages:
    - duration: 10s
      target: 10
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error for invalid mode")
	}
}

func TestValidate_MaxRPS_ArrivalRate_Error(t *testing.T) {
	yaml := `
load:
  mode: arrival_rate
  max_rps: 100
  stages:
    - duration: 10s
      target: 50
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error: max_rps not valid in arrival_rate mode")
	}
}

func TestValidate_MaxRPS_Negative_Error(t *testing.T) {
	yaml := `
load:
  max_rps: -1
  stages:
    - duration: 10s
      target: 10
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Error("expected validation error for negative max_rps")
	}
}

func TestValidate_Mode_DefaultIsVU(t *testing.T) {
	yaml := `
load:
  stages:
    - duration: 5s
      target: 1
endpoints:
  - url: "http://localhost"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Load.Mode != "vu" {
		t.Errorf("expected default mode 'vu', got %q", cfg.Load.Mode)
	}
}

func TestTotalDuration(t *testing.T) {
	cfg := &Config{
		Load: LoadConfig{
			Stages: []Stage{
				{Duration: Duration{10 * time.Second}},
				{Duration: Duration{30 * time.Second}},
				{Duration: Duration{10 * time.Second}},
			},
		},
	}
	expected := 50 * time.Second
	if got := cfg.TotalDuration(); got != expected {
		t.Errorf("expected %v, got %v", expected, got)
	}
}
