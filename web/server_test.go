package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFullFlow_Integration(t *testing.T) {
	// Create a mock target API
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer target.Close()

	// Set up the web UI server
	tmpl, err := LoadTemplates("templates")
	if err != nil {
		t.Fatalf("loading templates: %v", err)
	}
	state := NewState()
	srv := NewServer(":0", state, tmpl)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	// Step 1: GET / — should show dashboard
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Step 2: GET /configure — should show form
	resp, err = client.Get(ts.URL + "/configure")
	if err != nil {
		t.Fatalf("GET /configure: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /configure: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Step 3: POST /configure with action=run — should start test and redirect
	vals := url.Values{
		"action":                       {"run"},
		"name":                         {"Integration Test"},
		"mode":                         {"vu"},
		"load_style":                   {"shorthand"},
		"ramp_up":                      {"300ms"},
		"steady_state":                 {"300ms"},
		"ramp_down":                    {"300ms"},
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
	resp, err = client.PostForm(ts.URL+"/configure", vals)
	if err != nil {
		t.Fatalf("POST /configure: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("POST /configure: expected 303, got %d", resp.StatusCode)
	}
	testURL := resp.Header.Get("Location")
	if !strings.HasPrefix(testURL, "/test/") {
		t.Fatalf("expected redirect to /test/{id}, got %q", testURL)
	}
	resp.Body.Close()

	// Step 4: GET /test/{id} — should show running page
	resp, err = client.Get(ts.URL + testURL)
	if err != nil {
		t.Fatalf("GET %s: %v", testURL, err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET %s: expected 200, got %d", testURL, resp.StatusCode)
	}
	resp.Body.Close()

	// Step 5: Wait for test to complete
	testID := strings.TrimPrefix(testURL, "/test/")
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		tr := state.GetTest(testID)
		if tr != nil && tr.Status != "running" {
			break
		}
	}

	tr := state.GetTest(testID)
	if tr == nil {
		t.Fatal("test not found in state")
	}
	if tr.Status == "running" {
		t.Fatal("test did not complete in time")
	}

	// Step 6: GET /test/{id} — should show results (no meta-refresh)
	resp, err = client.Get(ts.URL + testURL)
	if err != nil {
		t.Fatalf("GET %s (results): %v", testURL, err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET %s (results): expected 200, got %d", testURL, resp.StatusCode)
	}
	resp.Body.Close()

	// Step 7: GET / — should show the completed test in recent list
	resp, err = client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / (after): %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET / (after): expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStopTest_Integration(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	tmpl, err := LoadTemplates("templates")
	if err != nil {
		t.Fatalf("loading templates: %v", err)
	}
	state := NewState()
	srv := NewServer(":0", state, tmpl)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Start a long test
	vals := url.Values{
		"action":                       {"run"},
		"name":                         {"Stop Test"},
		"mode":                         {"vu"},
		"load_style":                   {"shorthand"},
		"ramp_up":                      {"10s"},
		"steady_state":                 {"30s"},
		"ramp_down":                    {"10s"},
		"max_vus":                      {"2"},
		"timeout":                      {"5s"},
		"output_format":                {"console"},
		"output_interval":              {"5s"},
		"endpoints[0].name":            {"health"},
		"endpoints[0].method":          {"GET"},
		"endpoints[0].url":             {target.URL},
		"endpoints[0].weight":          {"1"},
		"endpoints[0].expect_status":   {"200"},
	}
	resp, err := client.PostForm(ts.URL+"/configure", vals)
	if err != nil {
		t.Fatalf("POST /configure: %v", err)
	}
	testURL := resp.Header.Get("Location")
	resp.Body.Close()

	testID := strings.TrimPrefix(testURL, "/test/")
	time.Sleep(200 * time.Millisecond)

	// Stop it
	resp, err = client.Get(ts.URL + testURL + "/stop")
	if err != nil {
		t.Fatalf("GET stop: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Wait for it to register as stopped
	time.Sleep(500 * time.Millisecond)
	tr := state.GetTest(testID)
	if tr == nil {
		t.Fatal("test not found")
	}
	if tr.Status == "running" {
		t.Error("test should not still be running after stop")
	}
}
