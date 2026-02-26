package web

import (
	"fmt"
	"net/http"
)

// NewServer creates an HTTP server with all routes registered.
func NewServer(addr string, state *State, templates *Templates) *http.Server {
	h := NewHandlers(state, templates)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", h.handleIndex)
	mux.HandleFunc("GET /configure", h.handleConfigure)
	mux.HandleFunc("POST /configure", h.handleConfigurePost)
	mux.HandleFunc("GET /test/{id}", h.handleTestStatus)
	mux.HandleFunc("GET /test/{id}/stop", h.handleTestStop)

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}

// ListenAndServe starts the web server.
func ListenAndServe(addr, templateDir string) error {
	templates, err := LoadTemplates(templateDir)
	if err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	state := NewState()
	srv := NewServer(addr, state, templates)

	fmt.Printf("perf-test web UI running at http://%s\n", addr)
	return srv.ListenAndServe()
}
