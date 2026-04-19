package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"time"

	"github.com/jvreagan/perf-test/internal/metrics"
	"github.com/jvreagan/perf-test/internal/reporter"
)

//go:embed templates/*.html
var embeddedTemplates embed.FS

// Templates holds parsed HTML templates, one per page.
type Templates struct {
	pages map[string]*template.Template
}

var funcMap = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"seq": func(n int) []int {
		s := make([]int, n)
		for i := range s {
			s[i] = i
		}
		return s
	},
	"fmtDuration": func(d metrics.JSONDuration) string { return reporter.FmtDur(d.Duration()) },
	"fmtElapsed":  func(d metrics.JSONDuration) string { return reporter.FormatElapsed(d.Duration()) },
	"fmtFloat":    func(f float64) string { return fmt.Sprintf("%.1f", f) },
	"sortedEndpoints": func(m map[string]*metrics.EndpointStats) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	},
	"fmtBytes": func(b int64) string { return reporter.FmtBytes(b) },
	"fmtTime":  func(t time.Time) string { return t.Format("2006-01-02 15:04:05") },
	"fmtPct": func(errors, total int64) string {
		if total == 0 {
			return "0.0"
		}
		return fmt.Sprintf("%.1f", float64(errors)/float64(total)*100)
	},
}

// LoadTemplates parses page templates from a directory, each paired with layout.html.
func LoadTemplates(dir string) (*Templates, error) {
	layoutFile := filepath.Join(dir, "layout.html")
	pageFiles := []string{"index.html", "configure.html", "running.html", "results.html"}

	pages := make(map[string]*template.Template)
	for _, page := range pageFiles {
		pageFile := filepath.Join(dir, page)
		tmpl, err := template.New("").Funcs(funcMap).ParseFiles(layoutFile, pageFile)
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", page, err)
		}
		pages[page] = tmpl
	}

	return &Templates{pages: pages}, nil
}

// LoadEmbeddedTemplates parses page templates from the embedded filesystem.
func LoadEmbeddedTemplates() (*Templates, error) {
	return loadTemplatesFromFS(embeddedTemplates, "templates")
}

func loadTemplatesFromFS(fsys fs.FS, dir string) (*Templates, error) {
	pageFiles := []string{"index.html", "configure.html", "running.html", "results.html"}

	layoutData, err := fs.ReadFile(fsys, dir+"/layout.html")
	if err != nil {
		return nil, fmt.Errorf("reading layout.html: %w", err)
	}

	pages := make(map[string]*template.Template)
	for _, page := range pageFiles {
		pageData, err := fs.ReadFile(fsys, dir+"/"+page)
		if err != nil {
			return nil, fmt.Errorf("reading template %s: %w", page, err)
		}
		tmpl := template.New("").Funcs(funcMap)
		if _, err := tmpl.Parse(string(layoutData)); err != nil {
			return nil, fmt.Errorf("parsing layout for %s: %w", page, err)
		}
		if _, err := tmpl.Parse(string(pageData)); err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", page, err)
		}
		pages[page] = tmpl
	}

	return &Templates{pages: pages}, nil
}

// Render executes a page template using the "layout" entry point.
func (t *Templates) Render(w io.Writer, name string, data interface{}) error {
	tmpl, ok := t.pages[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	return tmpl.ExecuteTemplate(w, "layout", data)
}

