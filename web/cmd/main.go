package main

import (
	"flag"
	"log"
	"path/filepath"
	"runtime"

	"github.com/jvreagan/perf-test/web"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "listen address")
	templateDir := flag.String("templates", "", "path to templates directory (default: auto-detect)")
	flag.Parse()

	tmplDir := *templateDir
	if tmplDir == "" {
		// Auto-detect relative to this source file
		_, filename, _, _ := runtime.Caller(0)
		tmplDir = filepath.Join(filepath.Dir(filename), "..", "templates")
	}

	if err := web.ListenAndServe(*addr, tmplDir); err != nil {
		log.Fatal(err)
	}
}
