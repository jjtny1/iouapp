package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaHandler serves the built single-page app from dir, falling back to
// index.html so client-side routes resolve. Before the frontend is built
// it returns a plain-text notice instead of a 404.
func spaHandler(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := filepath.Join(dir, "index.html")
		if _, err := os.Stat(index); err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("IOU API is running. Frontend not built yet — run `npm run build` in web/."))
			return
		}

		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if !strings.HasPrefix(path, filepath.Clean(dir)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			// Vite content-hashes everything under /assets, so those files
			// are safe to cache forever; anything else (index.html included)
			// must revalidate or a deploy leaves browsers on a stale UI.
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeFile(w, r, path)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
