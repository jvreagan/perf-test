package web

import (
	"fmt"
	"html/template"
	"io"
	"path/filepath"
	"time"
)

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
	"fmtDuration": formatDurationMS,
	"fmtElapsed":  formatElapsed,
	"fmtFloat":    func(f float64) string { return fmt.Sprintf("%.1f", f) },
	"fmtPct": func(errors, total int64) string {
		if total == 0 {
			return "0.0"
		}
		return fmt.Sprintf("%.1f", float64(errors)/float64(total)*100)
	},
}

// LoadTemplates parses page templates, each paired with layout.html.
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

// Render executes a page template using the "layout" entry point.
func (t *Templates) Render(w io.Writer, name string, data interface{}) error {
	tmpl, ok := t.pages[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	return tmpl.ExecuteTemplate(w, "layout", data)
}

func formatDurationMS(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fus", float64(d)/float64(time.Microsecond))
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func formatElapsed(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
