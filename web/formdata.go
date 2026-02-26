package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jvreagan/perf-test/internal/config"
)

// FormData holds all configuration form fields as strings.
type FormData struct {
	Name        string
	Description string

	// Load configuration
	Mode      string // "vu" or "arrival_rate"
	LoadStyle string // "shorthand" or "stages"
	ThinkTime string
	MaxRPS    string

	// Shorthand fields
	RampUp      string
	SteadyState string
	RampDown    string
	MaxVUs      string

	// Stages
	Stages []StageData

	// HTTP settings
	Timeout            string
	FollowRedirects    bool
	InsecureSkipVerify bool

	// Variables
	Variables []VariableData

	// Endpoints
	Endpoints []EndpointData

	// Output
	OutputFormat   string
	OutputInterval string
	OutputFile     string

	// Validation errors (set by server, not submitted)
	Errors []string
}

// StageData holds a single stage's form fields.
type StageData struct {
	Duration string
	Target   string
	Ramp     string
}

// VariableData holds a key-value pair form field.
type VariableData struct {
	Key   string
	Value string
}

// EndpointData holds a single endpoint's form fields.
type EndpointData struct {
	Name         string
	Method       string
	URL          string
	Body         string
	Weight       string
	ExpectStatus string
	Headers      []HeaderData
}

// HeaderData holds a single header key-value pair.
type HeaderData struct {
	Key   string
	Value string
}

// DefaultFormData returns a FormData with sensible defaults.
func DefaultFormData() *FormData {
	return &FormData{
		Mode:      "vu",
		LoadStyle: "shorthand",
		RampUp:    "10s",
		SteadyState: "30s",
		RampDown:  "10s",
		MaxVUs:    "10",
		Timeout:   "30s",
		OutputFormat:   "console",
		OutputInterval: "5s",
		Endpoints: []EndpointData{
			{
				Method:       "GET",
				Weight:       "1",
				ExpectStatus: "200",
			},
		},
	}
}

// ParseFormData extracts FormData from an HTTP request.
func ParseFormData(r *http.Request) *FormData {
	r.ParseForm()

	fd := &FormData{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		Mode:        r.FormValue("mode"),
		LoadStyle:   r.FormValue("load_style"),
		ThinkTime:   r.FormValue("think_time"),
		MaxRPS:      r.FormValue("max_rps"),
		RampUp:      r.FormValue("ramp_up"),
		SteadyState: r.FormValue("steady_state"),
		RampDown:    r.FormValue("ramp_down"),
		MaxVUs:      r.FormValue("max_vus"),
		Timeout:     r.FormValue("timeout"),
		FollowRedirects:    r.FormValue("follow_redirects") == "on",
		InsecureSkipVerify: r.FormValue("insecure_skip_verify") == "on",
		OutputFormat:   r.FormValue("output_format"),
		OutputInterval: r.FormValue("output_interval"),
		OutputFile:     r.FormValue("output_file"),
	}

	if fd.Mode == "" {
		fd.Mode = "vu"
	}
	if fd.LoadStyle == "" {
		fd.LoadStyle = "shorthand"
	}

	// Parse stages
	for i := 0; ; i++ {
		dur := r.FormValue(fmt.Sprintf("stages[%d].duration", i))
		if dur == "" && r.FormValue(fmt.Sprintf("stages[%d].target", i)) == "" {
			break
		}
		fd.Stages = append(fd.Stages, StageData{
			Duration: dur,
			Target:   r.FormValue(fmt.Sprintf("stages[%d].target", i)),
			Ramp:     r.FormValue(fmt.Sprintf("stages[%d].ramp", i)),
		})
	}

	// Parse variables
	for i := 0; ; i++ {
		key := r.FormValue(fmt.Sprintf("variables[%d].key", i))
		val := r.FormValue(fmt.Sprintf("variables[%d].value", i))
		if key == "" && val == "" {
			break
		}
		fd.Variables = append(fd.Variables, VariableData{Key: key, Value: val})
	}

	// Parse endpoints
	for i := 0; ; i++ {
		url := r.FormValue(fmt.Sprintf("endpoints[%d].url", i))
		method := r.FormValue(fmt.Sprintf("endpoints[%d].method", i))
		name := r.FormValue(fmt.Sprintf("endpoints[%d].name", i))
		if url == "" && method == "" && name == "" {
			break
		}
		ep := EndpointData{
			Name:         name,
			Method:       method,
			URL:          url,
			Body:         r.FormValue(fmt.Sprintf("endpoints[%d].body", i)),
			Weight:       r.FormValue(fmt.Sprintf("endpoints[%d].weight", i)),
			ExpectStatus: r.FormValue(fmt.Sprintf("endpoints[%d].expect_status", i)),
		}

		// Parse headers for this endpoint
		for j := 0; ; j++ {
			hk := r.FormValue(fmt.Sprintf("endpoints[%d].headers[%d].key", i, j))
			hv := r.FormValue(fmt.Sprintf("endpoints[%d].headers[%d].value", i, j))
			if hk == "" && hv == "" {
				break
			}
			ep.Headers = append(ep.Headers, HeaderData{Key: hk, Value: hv})
		}

		fd.Endpoints = append(fd.Endpoints, ep)
	}

	return fd
}

// ToConfig converts FormData to a config.Config, returning validation errors.
func (fd *FormData) ToConfig() (*config.Config, error) {
	cfg := &config.Config{
		Name:        fd.Name,
		Description: fd.Description,
	}

	// Load config
	cfg.Load.Mode = fd.Mode

	if fd.ThinkTime != "" {
		d, err := time.ParseDuration(fd.ThinkTime)
		if err != nil {
			return nil, fmt.Errorf("invalid think time %q: %w", fd.ThinkTime, err)
		}
		cfg.Load.ThinkTime = config.Duration{Duration: d}
	}

	if fd.MaxRPS != "" {
		f, err := strconv.ParseFloat(fd.MaxRPS, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid max RPS %q: %w", fd.MaxRPS, err)
		}
		cfg.Load.MaxRPS = f
	}

	if fd.LoadStyle == "stages" {
		for i, s := range fd.Stages {
			if s.Duration == "" && s.Target == "" {
				continue
			}
			d, err := time.ParseDuration(s.Duration)
			if err != nil {
				return nil, fmt.Errorf("stage %d: invalid duration %q: %w", i+1, s.Duration, err)
			}
			t, err := strconv.Atoi(s.Target)
			if err != nil {
				return nil, fmt.Errorf("stage %d: invalid target %q: %w", i+1, s.Target, err)
			}
			cfg.Load.Stages = append(cfg.Load.Stages, config.Stage{
				Duration: config.Duration{Duration: d},
				Target:   t,
				Ramp:     s.Ramp,
			})
		}
	} else {
		// Shorthand
		if fd.RampUp != "" {
			d, err := time.ParseDuration(fd.RampUp)
			if err != nil {
				return nil, fmt.Errorf("invalid ramp up %q: %w", fd.RampUp, err)
			}
			cfg.Load.RampUp = config.Duration{Duration: d}
		}
		if fd.SteadyState != "" {
			d, err := time.ParseDuration(fd.SteadyState)
			if err != nil {
				return nil, fmt.Errorf("invalid steady state %q: %w", fd.SteadyState, err)
			}
			cfg.Load.SteadyState = config.Duration{Duration: d}
		}
		if fd.RampDown != "" {
			d, err := time.ParseDuration(fd.RampDown)
			if err != nil {
				return nil, fmt.Errorf("invalid ramp down %q: %w", fd.RampDown, err)
			}
			cfg.Load.RampDown = config.Duration{Duration: d}
		}
		if fd.MaxVUs != "" {
			v, err := strconv.Atoi(fd.MaxVUs)
			if err != nil {
				return nil, fmt.Errorf("invalid max VUs %q: %w", fd.MaxVUs, err)
			}
			cfg.Load.MaxVUs = v
		}
	}

	// HTTP config
	if fd.Timeout != "" {
		d, err := time.ParseDuration(fd.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout %q: %w", fd.Timeout, err)
		}
		cfg.HTTP.Timeout = config.Duration{Duration: d}
	}
	cfg.HTTP.FollowRedirects = fd.FollowRedirects
	cfg.HTTP.InsecureSkipVerify = fd.InsecureSkipVerify

	// Variables
	if len(fd.Variables) > 0 {
		cfg.Variables = make(map[string]string)
		for _, v := range fd.Variables {
			if v.Key != "" {
				cfg.Variables[v.Key] = v.Value
			}
		}
	}

	// Endpoints
	for _, ep := range fd.Endpoints {
		e := config.Endpoint{
			Name:   ep.Name,
			Method: ep.Method,
			URL:    ep.URL,
			Body:   ep.Body,
		}
		if ep.Weight != "" {
			w, err := strconv.Atoi(ep.Weight)
			if err != nil {
				return nil, fmt.Errorf("endpoint %q: invalid weight %q: %w", ep.Name, ep.Weight, err)
			}
			e.Weight = w
		}
		if ep.ExpectStatus != "" {
			s, err := strconv.Atoi(ep.ExpectStatus)
			if err != nil {
				return nil, fmt.Errorf("endpoint %q: invalid expected status %q: %w", ep.Name, ep.ExpectStatus, err)
			}
			e.Expect.Status = s
		}
		if len(ep.Headers) > 0 {
			e.Headers = make(map[string]string)
			for _, h := range ep.Headers {
				if h.Key != "" {
					e.Headers[h.Key] = h.Value
				}
			}
		}
		cfg.Endpoints = append(cfg.Endpoints, e)
	}

	// Output config
	if fd.OutputFormat != "" {
		cfg.Output.Format = fd.OutputFormat
	}
	if fd.OutputInterval != "" {
		d, err := time.ParseDuration(fd.OutputInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid output interval %q: %w", fd.OutputInterval, err)
		}
		cfg.Output.Interval = config.Duration{Duration: d}
	}
	cfg.Output.File = fd.OutputFile

	// Apply defaults and normalize (same as config.Load does after YAML parse)
	cfg.ApplyDefaults()
	cfg.NormalizeStages()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// TotalDurationStr returns a human-friendly duration string from the form data.
func (fd *FormData) TotalDurationStr() string {
	if fd.LoadStyle == "stages" {
		var total time.Duration
		for _, s := range fd.Stages {
			d, err := time.ParseDuration(s.Duration)
			if err == nil {
				total += d
			}
		}
		return total.String()
	}
	var total time.Duration
	for _, s := range []string{fd.RampUp, fd.SteadyState, fd.RampDown} {
		d, err := time.ParseDuration(s)
		if err == nil {
			total += d
		}
	}
	return total.String()
}

// ModeLabel returns a human-friendly label for the load mode.
func (fd *FormData) ModeLabel() string {
	if fd.Mode == "arrival_rate" {
		return "Arrival Rate (fixed RPS)"
	}
	return "Virtual Users (VU pool)"
}

// TargetLabel returns "VUs" or "RPS" based on mode.
func TargetLabel(mode string) string {
	if strings.EqualFold(mode, "arrival_rate") {
		return "RPS"
	}
	return "VUs"
}
