package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a custom type to support YAML time.Duration parsing.
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = dur
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		var ns int64
		if err := json.Unmarshal(b, &ns); err != nil {
			return err
		}
		d.Duration = time.Duration(ns)
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

// Stage represents a single load stage.
type Stage struct {
	Duration Duration `yaml:"duration" json:"duration"`
	Target   int      `yaml:"target" json:"target"`
	Ramp     string   `yaml:"ramp" json:"ramp,omitempty"` // "linear" (default) or "step"
}

// LoadConfig holds the load profile configuration.
type LoadConfig struct {
	Mode        string   `yaml:"mode" json:"mode"`
	Stages      []Stage  `yaml:"stages" json:"stages"`
	RampUp      Duration `yaml:"ramp_up" json:"ramp_up,omitempty"`
	SteadyState Duration `yaml:"steady_state" json:"steady_state,omitempty"`
	RampDown    Duration `yaml:"ramp_down" json:"ramp_down,omitempty"`
	MaxVUs      int      `yaml:"max_vus" json:"max_vus,omitempty"`
	MaxRPS      float64  `yaml:"max_rps" json:"max_rps,omitempty"`
	ThinkTime   Duration `yaml:"think_time" json:"think_time,omitempty"`
}

// HTTPConfig holds HTTP client settings.
type HTTPConfig struct {
	Timeout            Duration `yaml:"timeout" json:"timeout"`
	FollowRedirects    bool     `yaml:"follow_redirects" json:"follow_redirects"`
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
}

// ExpectConfig holds response expectations.
type ExpectConfig struct {
	Status int `yaml:"status" json:"status"`
}

// Endpoint defines a single HTTP endpoint to test.
type Endpoint struct {
	Name    string            `yaml:"name" json:"name"`
	Method  string            `yaml:"method" json:"method"`
	URL     string            `yaml:"url" json:"url"`
	Headers map[string]string `yaml:"headers" json:"headers,omitempty"`
	Body    string            `yaml:"body" json:"body,omitempty"`
	Weight  int               `yaml:"weight" json:"weight"`
	Expect  ExpectConfig      `yaml:"expect" json:"expect"`
}

// OutputConfig defines reporting settings.
type OutputConfig struct {
	Format   string   `yaml:"format" json:"format"`
	Interval Duration `yaml:"interval" json:"interval"`
	File     string   `yaml:"file" json:"file,omitempty"`
}

// ThresholdConfig holds pass/fail criteria for the test.
type ThresholdConfig struct {
	P95        Duration `yaml:"p95" json:"p95,omitempty"`
	P99        Duration `yaml:"p99" json:"p99,omitempty"`
	MaxLatency Duration `yaml:"max_latency" json:"max_latency,omitempty"`
	ErrorRate  float64  `yaml:"error_rate" json:"error_rate,omitempty"`
	MinRPS     float64  `yaml:"min_rps" json:"min_rps,omitempty"`
}

// Config is the top-level configuration structure.
type Config struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description,omitempty"`
	Load        LoadConfig        `yaml:"load" json:"load"`
	HTTP        HTTPConfig        `yaml:"http" json:"http"`
	Variables   map[string]string `yaml:"variables" json:"variables,omitempty"`
	Endpoints   []Endpoint        `yaml:"endpoints" json:"endpoints"`
	Output      OutputConfig      `yaml:"output" json:"output"`
	Thresholds  ThresholdConfig   `yaml:"thresholds" json:"thresholds,omitempty"`
}

// Load reads a config file, parses YAML, expands environment variables only
// in the variables section, applies defaults, normalizes stages, and validates.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Expand environment variables only in the variables section values.
	// This allows ${TOKEN} in variables to resolve from the OS environment
	// without clobbering ${base_url} template tokens in URLs/bodies.
	for k, v := range cfg.Variables {
		cfg.Variables[k] = os.ExpandEnv(v)
	}

	cfg.ApplyDefaults()
	cfg.NormalizeStages()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// ApplyDefaults sets sensible defaults for unspecified fields.
func (c *Config) ApplyDefaults() {
	if c.Load.Mode == "" {
		c.Load.Mode = "vu"
	}
	if c.HTTP.Timeout.Duration == 0 {
		c.HTTP.Timeout = Duration{30 * time.Second}
	}
	// Note: FollowRedirects defaults to false (Go zero value).
	// This is intentional for load testing — explicit redirects give more
	// accurate latency measurements and avoid unexpected request multiplication.
	if c.Output.Format == "" {
		c.Output.Format = "console"
	}
	if c.Output.Interval.Duration == 0 {
		c.Output.Interval = Duration{5 * time.Second}
	}
	for i := range c.Endpoints {
		if c.Endpoints[i].Method == "" {
			c.Endpoints[i].Method = "GET"
		}
		if c.Endpoints[i].Weight == 0 {
			c.Endpoints[i].Weight = 1
		}
		if c.Endpoints[i].Expect.Status == 0 {
			c.Endpoints[i].Expect.Status = 200
		}
		if c.Endpoints[i].Name == "" {
			c.Endpoints[i].Name = fmt.Sprintf("%s %s", c.Endpoints[i].Method, c.Endpoints[i].URL)
		}
	}
}

// NormalizeStages converts simple shorthand (ramp_up/steady_state/ramp_down) into stages.
func (c *Config) NormalizeStages() {
	if len(c.Load.Stages) > 0 {
		return
	}
	if c.Load.MaxVUs == 0 {
		return
	}

	var stages []Stage
	if c.Load.RampUp.Duration > 0 {
		stages = append(stages, Stage{
			Duration: c.Load.RampUp,
			Target:   c.Load.MaxVUs,
		})
	}
	if c.Load.SteadyState.Duration > 0 {
		stages = append(stages, Stage{
			Duration: c.Load.SteadyState,
			Target:   c.Load.MaxVUs,
		})
	}
	if c.Load.RampDown.Duration > 0 {
		stages = append(stages, Stage{
			Duration: c.Load.RampDown,
			Target:   0,
		})
	}
	c.Load.Stages = stages
}

// Validate checks that the config has all required fields.
func (c *Config) Validate() error {
	if len(c.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint is required")
	}
	validMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true,
		"PATCH": true, "HEAD": true, "OPTIONS": true,
	}
	for i, ep := range c.Endpoints {
		if strings.TrimSpace(ep.URL) == "" {
			return fmt.Errorf("endpoint[%d] %q: URL is required", i, ep.Name)
		}
		if !validMethods[ep.Method] {
			return fmt.Errorf("endpoint[%d] %q: invalid HTTP method %q", i, ep.Name, ep.Method)
		}
		if ep.Weight < 0 {
			return fmt.Errorf("endpoint[%d] %q: weight must be >= 0", i, ep.Name)
		}
	}
	validModes := map[string]bool{"vu": true, "arrival_rate": true}
	if !validModes[c.Load.Mode] {
		return fmt.Errorf("load.mode must be \"vu\" or \"arrival_rate\" (got %q)", c.Load.Mode)
	}
	if c.Load.MaxRPS < 0 {
		return fmt.Errorf("load.max_rps must be >= 0")
	}
	if c.Load.MaxRPS > 0 && c.Load.Mode == "arrival_rate" {
		return fmt.Errorf("load.max_rps is only valid in vu mode")
	}
	if c.Load.ThinkTime.Duration < 0 {
		return fmt.Errorf("load.think_time must be >= 0")
	}
	if c.HTTP.Timeout.Duration < 0 {
		return fmt.Errorf("http.timeout must be >= 0")
	}
	if len(c.Load.Stages) == 0 {
		return fmt.Errorf("load stages are required (use stages or ramp_up/steady_state/ramp_down with max_vus)")
	}
	targetLabel := "VUs"
	if c.Load.Mode == "arrival_rate" {
		targetLabel = "RPS"
	}
	for i, s := range c.Load.Stages {
		if s.Duration.Duration <= 0 {
			return fmt.Errorf("stage[%d]: duration must be positive", i)
		}
		if s.Target < 0 {
			return fmt.Errorf("stage[%d]: target %s must be >= 0", i, targetLabel)
		}
		if s.Ramp != "" && s.Ramp != "linear" && s.Ramp != "step" {
			return fmt.Errorf("stage[%d]: ramp must be \"linear\" or \"step\" (got %q)", i, s.Ramp)
		}
	}
	// Check for duplicate endpoint names
	seenNames := make(map[string]int)
	for i, ep := range c.Endpoints {
		if prev, ok := seenNames[ep.Name]; ok {
			return fmt.Errorf("endpoint[%d] has duplicate name %q (same as endpoint[%d])", i, ep.Name, prev)
		}
		seenNames[ep.Name] = i
	}

	validFormats := map[string]bool{"console": true, "json": true}
	if !validFormats[c.Output.Format] {
		return fmt.Errorf("output.format must be one of: console, json (got %q)", c.Output.Format)
	}
	if c.Output.Interval.Duration <= 0 {
		return fmt.Errorf("output.interval must be positive (got %s)", c.Output.Interval.Duration)
	}
	if c.Thresholds.ErrorRate < 0 || c.Thresholds.ErrorRate > 100 {
		return fmt.Errorf("thresholds.error_rate must be between 0 and 100 (got %.1f)", c.Thresholds.ErrorRate)
	}
	if c.Thresholds.MinRPS < 0 {
		return fmt.Errorf("thresholds.min_rps must be >= 0 (got %.1f)", c.Thresholds.MinRPS)
	}
	return nil
}

// HasThresholds returns true if any threshold is configured.
func (tc ThresholdConfig) HasThresholds() bool {
	return tc.P95.Duration > 0 || tc.P99.Duration > 0 || tc.MaxLatency.Duration > 0 ||
		tc.ErrorRate > 0 || tc.MinRPS > 0
}

// TotalDuration returns the sum of all stage durations.
func (c *Config) TotalDuration() time.Duration {
	var total time.Duration
	for _, s := range c.Load.Stages {
		total += s.Duration.Duration
	}
	if total == 0 {
		total = c.Load.RampUp.Duration + c.Load.SteadyState.Duration + c.Load.RampDown.Duration
	}
	return total
}
