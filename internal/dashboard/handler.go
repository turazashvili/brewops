package dashboard

import (
	"net/http"
	"os"
	"path/filepath"
)

// Handler serves the web dashboard static files.
// In a more serious project, we'd embed these with go:embed.
// But we're not serious. We're making coffee.
func Handler(webDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve index.html for /dashboard or /dashboard/
		path := r.URL.Path
		if path == "/dashboard" || path == "/dashboard/" {
			http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
			return
		}

		// Serve static files from web directory
		file := filepath.Join(webDir, filepath.Base(path))
		if _, err := os.Stat(file); err == nil {
			http.ServeFile(w, r, file)
			return
		}

		// Fallback to index.html
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})
}
