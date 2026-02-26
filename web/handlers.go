package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jvreagan/perf-test/internal/metrics"
)

// Handlers holds dependencies for HTTP handlers.
type Handlers struct {
	state     *State
	templates *Templates
}

// NewHandlers creates a Handlers with the given state and templates.
func NewHandlers(state *State, templates *Templates) *Handlers {
	return &Handlers{state: state, templates: templates}
}

func (h *Handlers) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Active": h.state.ActiveTest(),
		"Recent": h.state.RecentTests(20),
	}
	h.render(w, "index.html", data)
}

func (h *Handlers) handleConfigure(w http.ResponseWriter, r *http.Request) {
	if active := h.state.ActiveTest(); active != nil {
		http.Redirect(w, r, "/test/"+active.ID, http.StatusSeeOther)
		return
	}
	data := map[string]interface{}{
		"FormData": DefaultFormData(),
	}
	h.render(w, "configure.html", data)
}

func (h *Handlers) handleConfigurePost(w http.ResponseWriter, r *http.Request) {
	fd := ParseFormData(r)
	action := r.FormValue("action")

	switch {
	case action == "add_endpoint":
		fd.Endpoints = append(fd.Endpoints, EndpointData{
			Method: "GET", Weight: "1", ExpectStatus: "200",
		})
		h.renderConfigure(w, fd)
		return

	case strings.HasPrefix(action, "remove_endpoint_"):
		idx := parseActionIndex(action, "remove_endpoint_")
		if idx >= 0 && idx < len(fd.Endpoints) {
			fd.Endpoints = append(fd.Endpoints[:idx], fd.Endpoints[idx+1:]...)
		}
		h.renderConfigure(w, fd)
		return

	case action == "add_stage":
		fd.Stages = append(fd.Stages, StageData{Ramp: "linear"})
		h.renderConfigure(w, fd)
		return

	case strings.HasPrefix(action, "remove_stage_"):
		idx := parseActionIndex(action, "remove_stage_")
		if idx >= 0 && idx < len(fd.Stages) {
			fd.Stages = append(fd.Stages[:idx], fd.Stages[idx+1:]...)
		}
		h.renderConfigure(w, fd)
		return

	case action == "add_variable":
		fd.Variables = append(fd.Variables, VariableData{})
		h.renderConfigure(w, fd)
		return

	case strings.HasPrefix(action, "remove_variable_"):
		idx := parseActionIndex(action, "remove_variable_")
		if idx >= 0 && idx < len(fd.Variables) {
			fd.Variables = append(fd.Variables[:idx], fd.Variables[idx+1:]...)
		}
		h.renderConfigure(w, fd)
		return

	case strings.HasPrefix(action, "add_header_"):
		idx := parseActionIndex(action, "add_header_")
		if idx >= 0 && idx < len(fd.Endpoints) {
			fd.Endpoints[idx].Headers = append(fd.Endpoints[idx].Headers, HeaderData{})
		}
		h.renderConfigure(w, fd)
		return

	case strings.HasPrefix(action, "remove_header_"):
		// Format: remove_header_<epIdx>_<hdrIdx>
		parts := strings.SplitN(strings.TrimPrefix(action, "remove_header_"), "_", 2)
		if len(parts) == 2 {
			epIdx, _ := strconv.Atoi(parts[0])
			hdrIdx, _ := strconv.Atoi(parts[1])
			if epIdx >= 0 && epIdx < len(fd.Endpoints) {
				headers := fd.Endpoints[epIdx].Headers
				if hdrIdx >= 0 && hdrIdx < len(headers) {
					fd.Endpoints[epIdx].Headers = append(headers[:hdrIdx], headers[hdrIdx+1:]...)
				}
			}
		}
		h.renderConfigure(w, fd)
		return

	case action == "switch_load_style":
		// Toggle between shorthand and stages, preserving data
		if fd.LoadStyle == "shorthand" {
			fd.LoadStyle = "stages"
			if len(fd.Stages) == 0 {
				fd.Stages = []StageData{{Duration: "30s", Target: "10", Ramp: "linear"}}
			}
		} else {
			fd.LoadStyle = "shorthand"
		}
		h.renderConfigure(w, fd)
		return

	case action == "run":
		if active := h.state.ActiveTest(); active != nil {
			fd.Errors = append(fd.Errors, "A test is already running. Stop it first or wait for it to complete.")
			h.renderConfigure(w, fd)
			return
		}

		cfg, err := fd.ToConfig()
		if err != nil {
			fd.Errors = append(fd.Errors, err.Error())
			h.renderConfigure(w, fd)
			return
		}

		run := h.state.StartTest(cfg)
		if run == nil {
			fd.Errors = append(fd.Errors, "Failed to start test. A test may already be running.")
			h.renderConfigure(w, fd)
			return
		}

		http.Redirect(w, r, "/test/"+run.ID, http.StatusSeeOther)
		return
	}

	// Unknown action, just re-render
	h.renderConfigure(w, fd)
}

func (h *Handlers) handleTestStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tr := h.state.GetTest(id)
	if tr == nil {
		http.NotFound(w, r)
		return
	}

	if tr.Status == "running" {
		var stats *metrics.Stats
		if collector := tr.Engine.Collector(); collector != nil {
			stats = collector.Snapshot()
		}

		totalDur := tr.Config.TotalDuration()
		elapsed := time.Since(tr.StartedAt)
		pct := 0.0
		if totalDur > 0 {
			pct = float64(elapsed) / float64(totalDur) * 100
			if pct > 100 {
				pct = 100
			}
		}

		data := map[string]interface{}{
			"TestRun":       tr,
			"Stats":         stats,
			"TotalDuration": totalDur,
			"ProgressPct":   fmt.Sprintf("%.0f", pct),
		}
		h.render(w, "running.html", data)
		return
	}

	// Completed/failed/stopped
	data := map[string]interface{}{
		"TestRun": tr,
		"Stats":   tr.FinalStats,
	}
	h.render(w, "results.html", data)
}

func (h *Handlers) handleTestStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.state.StopTest(id)
	// Give the goroutine a moment to clean up
	time.Sleep(500 * time.Millisecond)
	http.Redirect(w, r, "/test/"+id, http.StatusSeeOther)
}

func (h *Handlers) renderConfigure(w http.ResponseWriter, fd *FormData) {
	data := map[string]interface{}{
		"FormData": fd,
	}
	h.render(w, "configure.html", data)
}

func (h *Handlers) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.Render(w, name, data); err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
	}
}

func parseActionIndex(action, prefix string) int {
	s := strings.TrimPrefix(action, prefix)
	idx, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return idx
}
