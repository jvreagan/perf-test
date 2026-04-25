package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// NewServer creates an HTTP server with all routes registered.
func NewServer(addr string, state *State, templates *Templates) *http.Server {
	h := NewHandlers(state, templates)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", h.handleIndex)
	mux.HandleFunc("GET /configure", h.handleConfigure)
	mux.HandleFunc("POST /configure", h.handleConfigurePost)
	mux.HandleFunc("GET /test/{id}", h.handleTestStatus)
	mux.HandleFunc("POST /test/{id}/stop", h.handleTestStop)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// ListenAndServe starts the web server with graceful shutdown on SIGINT/SIGTERM.
// If templateDir is empty, embedded templates are used.
func ListenAndServe(addr, templateDir string) error {
	var templates *Templates
	var err error
	if templateDir != "" {
		templates, err = LoadTemplates(templateDir)
	} else {
		templates, err = LoadEmbeddedTemplates()
	}
	if err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	state := NewState()
	srv := NewServer(addr, state, templates)

	// Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("perf-test web UI running at http://%s\n", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		signal.Stop(sigCh)
		fmt.Println("\nShutting down gracefully...")
		// Stop any active test
		if active := state.ActiveTest(); active != nil {
			state.StopTest(active.ID)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
