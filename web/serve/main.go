// Command serve serves a browser build (what web/build.sh writes):
//
//	go run ./web/serve -dir build/web/cube [-addr :8080]
package main

import (
	"flag"
	"log"
	"net/http"
	"strings"
)

func main() {
	dir := flag.String("dir", "build/web", "directory to serve")
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	files := http.FileServer(http.Dir(*dir))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Revalidate every request, so a rebuild shows up on reload.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	log.Printf("serving %s on http://%s", *dir, host)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
