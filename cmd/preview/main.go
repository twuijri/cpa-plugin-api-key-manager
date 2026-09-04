// Local-only preview uses the same management handlers, never real provider credentials.
package main

import (
	"crypto/subtle"
	"flag"
	"fmt"
	"io"
	"log"
	"miftah.local/plugin/internal/bridge"
	"miftah.local/plugin/internal/core"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	path := flag.String("state", "data/preview/state.json", "preview state")
	flag.Parse()
	secret := os.Getenv("MIFTAH_PREVIEW_TOKEN")
	if len(secret) < 16 {
		log.Fatal("set MIFTAH_PREVIEW_TOKEN to 16+ characters")
	}
	s, err := core.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	a := bridge.App{Store: s}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/v0/resource/plugins/miftah/console", 302)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v0/management/") {
			given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(given), []byte(secret)) != 1 {
				http.Error(w, "unauthorized", 401)
				return
			}
		} else if !strings.HasPrefix(r.URL.Path, "/v0/resource/plugins/miftah/") {
			http.NotFound(w, r)
			return
		}
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "too large", 413)
			return
		}
		res := a.Manage(bridge.Request{Method: r.Method, Path: r.URL.Path, Headers: r.Header, Body: b})
		for k, v := range res.Headers {
			w.Header()[k] = v
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(res.Body)
	})
	fmt.Println("Miftah preview: http://127.0.0.1:8741 (no upstream connected)")
	log.Fatal((&http.Server{Addr: "127.0.0.1:8741", Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
