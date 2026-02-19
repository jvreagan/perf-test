package config

import (
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

// Stage represents a single load stage.
type Stage struct {
	Duration Duration `yaml:"duration"`
	Target   int      `yaml:"target"`
	Ramp     string   `yaml:"ramp"` // "linear" (default) or "step"
}

// LoadConfig holds the load profile configuration.
type LoadConfig struct {
	Mode        string   `yaml:"mode"` // "vu" (default) or "arrival_rate"
	Stages      []Stage  `yaml:"stages"`
	RampUp      Duration `yaml:"ramp_up"`
	SteadyState Duration `yaml:"steady_state"`
	RampDown    Duration `yaml:"ramp_down"`
	MaxVUs      int      `yaml:"max_vus"`
	MaxRPS      float64  `yaml:"max_rps"`
	ThinkTime   Duration `yaml:"think_time"`
}

// HTTPConfig holds HTTP client settings.
type HTTPConfig struct {
	Timeout            Duration `yaml:"timeout"`
	FollowRedirects    bool     `yaml:"follow_redirects"`
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify"`
}

// ExpectConfig holds response expectations.
type ExpectConfig struct {
	Status int `yaml:"status"`
}

// Endpoint defines a single HTTP endpoint to test.
type Endpoint struct {
	Name    string            `yaml:"name"`
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
	Weight  int               `yaml:"weight"`
	Expect  ExpectConfig      `yaml:"expect"`
}

// OutputConfig defines reporting settings.
type OutputConfig struct {
	Format   string   `yaml:"format"`
	Interval Duration `yaml:"interval"`
	File     string   `yaml:"file"`
}

// Config is the top-level configuration structure.
type Config struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Load        LoadConfig        `yaml:"load"`
	HTTP        HTTPConfig        `yaml:"http"`
	Variables   map[string]string `yaml:"variables"`
	Endpoints   []Endpoint        `yaml:"endpoints"`
	Output      OutputConfig      `yaml:"output"`
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

	cfg.applyDefaults()
	cfg.normalizeStages()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// applyDefaults sets sensible defaults for unspecified fields.
func (c *Config) applyDefaults() {
	if c.Load.Mode == "" {
		c.Load.Mode = "vu"
	}
	if c.HTTP.Timeout.Duration == 0 {
		c.HTTP.Timeout = Duration{30 * time.Second}
	}
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
	}
}

// normalizeStages converts simple shorthand (ramp_up/steady_state/ramp_down) into stages.
func (c *Config) normalizeStages() {
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
	for i, ep := range c.Endpoints {
		if strings.TrimSpace(ep.URL) == "" {
			return fmt.Errorf("endpoint[%d] %q: URL is required", i, ep.Name)
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
	validFormats := map[string]bool{"console": true, "json": true, "csv": true}
	if !validFormats[c.Output.Format] {
		return fmt.Errorf("output.format must be one of: console, json, csv (got %q)", c.Output.Format)
	}
	return nil
}

// TotalDuration returns the sum of all stage durations.
func (c *Config) TotalDuration() time.Duration {
	var total time.Duration
	for _, s := range c.Load.Stages {
		total += s.Duration.Duration
	}
	return total
}
