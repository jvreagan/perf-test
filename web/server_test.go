package web

import (
	"io"
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
		if tr != nil && tr.GetStatus() != "running" {
			break
		}
	}

	tr := state.GetTest(testID)
	if tr == nil {
		t.Fatal("test not found in state")
	}
	if tr.GetStatus() == "running" {
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
	resp, err = client.Post(ts.URL+testURL+"/stop", "", nil)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
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
	if tr.GetStatus() == "running" {
		t.Error("test should not still be running after stop")
	}
}

// --- Helper to set up a full integration server ---

func setupIntegrationServer(t *testing.T, targetURL string) (*httptest.Server, *State, *http.Client) {
	t.Helper()
	tmpl, err := LoadTemplates("templates")
	if err != nil {
		t.Fatalf("loading templates: %v", err)
	}
	state := NewState()
	srv := NewServer(":0", state, tmpl)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return ts, state, client
}

func startTestViaAPI(t *testing.T, ts *httptest.Server, client *http.Client, targetURL string, rampUp, steady, rampDown string, maxVUs string) string {
	t.Helper()
	vals := url.Values{
		"action":                     {"run"},
		"name":                       {"API Test"},
		"mode":                       {"vu"},
		"load_style":                 {"shorthand"},
		"ramp_up":                    {rampUp},
		"steady_state":               {steady},
		"ramp_down":                  {rampDown},
		"max_vus":                    {maxVUs},
		"timeout":                    {"5s"},
		"output_format":              {"console"},
		"output_interval":            {"10s"},
		"endpoints[0].name":          {"ep"},
		"endpoints[0].method":        {"GET"},
		"endpoints[0].url":           {targetURL},
		"endpoints[0].weight":        {"1"},
		"endpoints[0].expect_status": {"200"},
	}
	resp, err := client.PostForm(ts.URL+"/configure", vals)
	if err != nil {
		t.Fatalf("POST /configure: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /configure: expected 303, got %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

// --- Running page tests ---

func TestRunningPage_ShowsInstantRPS(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer target.Close()

	ts, _, client := setupIntegrationServer(t, target.URL)
	testURL := startTestViaAPI(t, ts, client, target.URL, "500ms", "1s", "500ms", "3")

	// Wait for some results to accumulate
	time.Sleep(300 * time.Millisecond)

	// GET running page
	resp, err := client.Get(ts.URL + testURL)
	if err != nil {
		t.Fatalf("GET %s: %v", testURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Should contain the RPS label and meta refresh
	if !strings.Contains(bodyStr, "RPS") {
		t.Error("running page should show RPS label")
	}
	if !strings.Contains(bodyStr, "meta http-equiv") {
		t.Error("running page should have meta refresh for auto-update")
	}
	if !strings.Contains(bodyStr, "running") {
		t.Error("running page should show running badge")
	}
}

func TestResultsPage_UsesAccessors(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer target.Close()

	ts, state, client := setupIntegrationServer(t, target.URL)
	testURL := startTestViaAPI(t, ts, client, target.URL, "200ms", "200ms", "200ms", "1")
	testID := strings.TrimPrefix(testURL, "/test/")

	// Wait for completion
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		tr := state.GetTest(testID)
		if tr != nil && tr.GetStatus() != "running" {
			break
		}
	}

	tr := state.GetTest(testID)
	if tr == nil {
		t.Fatal("test not found")
	}
	if tr.GetStatus() == "running" {
		t.Fatal("test did not complete")
	}

	// GET results page
	resp, err := client.Get(ts.URL + testURL)
	if err != nil {
		t.Fatalf("GET results: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Should show results content (not meta refresh)
	if strings.Contains(bodyStr, "meta http-equiv") {
		t.Error("results page should NOT have meta refresh")
	}
	if !strings.Contains(bodyStr, "Total Requests") {
		t.Error("results page should show Total Requests")
	}
	if !strings.Contains(bodyStr, "Avg RPS") {
		t.Error("results page should show Avg RPS")
	}
	// Status badge from GetStatus()
	if !strings.Contains(bodyStr, "completed") && !strings.Contains(bodyStr, "failed") {
		t.Error("results page should show completion badge")
	}
}

func TestStopEndpoint_ImmediateRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	ts, _, client := setupIntegrationServer(t, target.URL)
	testURL := startTestViaAPI(t, ts, client, target.URL, "10s", "30s", "10s", "2")

	time.Sleep(200 * time.Millisecond)

	// Time the stop endpoint — should be fast (no 500ms sleep)
	start := time.Now()
	resp, err := client.Post(ts.URL+testURL+"/stop", "", nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", resp.StatusCode)
	}

	// Should complete in under 200ms (was 500ms+ with the sleep)
	if elapsed > 400*time.Millisecond {
		t.Errorf("stop took %v — expected no sleep delay", elapsed)
	}
}

func TestDashboard_ShowsCompletedTestWithStats(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer target.Close()

	ts, state, client := setupIntegrationServer(t, target.URL)
	testURL := startTestViaAPI(t, ts, client, target.URL, "200ms", "200ms", "200ms", "1")
	testID := strings.TrimPrefix(testURL, "/test/")

	// Wait for completion
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		tr := state.GetTest(testID)
		if tr != nil && tr.GetStatus() != "running" {
			break
		}
	}

	// Dashboard should show the completed test
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "API Test") {
		t.Error("dashboard should show test name")
	}
	if !strings.Contains(bodyStr, "requests") {
		t.Error("dashboard should show request count from GetFinalStats()")
	}
}

func TestConfigure_RedirectsWhenTestRunning(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	ts, _, client := setupIntegrationServer(t, target.URL)
	testURL := startTestViaAPI(t, ts, client, target.URL, "10s", "30s", "10s", "1")

	// GET /configure should redirect to the running test
	resp, err := client.Get(ts.URL + "/configure")
	if err != nil {
		t.Fatalf("GET /configure: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != testURL {
		t.Errorf("expected redirect to %s, got %s", testURL, loc)
	}

	// Clean up
	client.Post(ts.URL+testURL+"/stop", "", nil)
}

func TestTestStatus_404ForNonexistent(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	ts, _, client := setupIntegrationServer(t, target.URL)

	resp, err := client.Get(ts.URL + "/test/nonexistent")
	if err != nil {
		t.Fatalf("GET /test/nonexistent: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestArrivalRate_FullFlow(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
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

	// Start an arrival rate test
	vals := url.Values{
		"action":                     {"run"},
		"name":                       {"Arrival Rate Test"},
		"mode":                       {"arrival_rate"},
		"load_style":                 {"shorthand"},
		"ramp_up":                    {"300ms"},
		"steady_state":               {"300ms"},
		"ramp_down":                  {"300ms"},
		"max_vus":                    {"20"},
		"timeout":                    {"5s"},
		"output_format":              {"console"},
		"output_interval":            {"10s"},
		"endpoints[0].name":          {"ep"},
		"endpoints[0].method":        {"GET"},
		"endpoints[0].url":           {target.URL},
		"endpoints[0].weight":        {"1"},
		"endpoints[0].expect_status": {"200"},
	}
	resp, err := client.PostForm(ts.URL+"/configure", vals)
	if err != nil {
		t.Fatalf("POST /configure: %v", err)
	}
	testURL := resp.Header.Get("Location")
	resp.Body.Close()

	testID := strings.TrimPrefix(testURL, "/test/")

	// Wait for completion
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		tr := state.GetTest(testID)
		if tr != nil && tr.GetStatus() != "running" {
			break
		}
	}

	tr := state.GetTest(testID)
	if tr == nil {
		t.Fatal("test not found")
	}
	if tr.GetStatus() == "running" {
		t.Fatal("arrival rate test did not complete in time")
	}

	stats := tr.GetFinalStats()
	if stats == nil {
		t.Fatal("expected non-nil FinalStats")
	}
	if stats.TotalRequests == 0 {
		t.Error("expected at least some requests in arrival rate test")
	}
}

func TestStopNonRunningTest(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()

	ts, _, client := setupIntegrationServer(t, target.URL)

	// Stop a non-existent test — should redirect gracefully, not panic
	resp, err := client.Post(ts.URL+"/test/nonexistent/stop", "", nil)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", resp.StatusCode)
	}
}
