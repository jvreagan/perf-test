package main

import (
	"flag"
	"log"

	"github.com/jvreagan/perf-test/web"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "listen address")
	templateDir := flag.String("templates", "", "path to templates directory (default: use embedded templates)")
	flag.Parse()

	if err := web.ListenAndServe(*addr, *templateDir); err != nil {
		log.Fatal(err)
	}
}
