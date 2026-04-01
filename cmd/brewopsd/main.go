package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/niko/brewops/internal/htcpcp"
	"github.com/niko/brewops/internal/metrics"
)

const banner = `
    ____                    ____            
   / __ )_________ _      / __ \____  _____
  / __  / ___/ __ \ | /| / / / / __ \/ ___/
 / /_/ / /  / /_/ / |/ |/ / /_/ / /_/ (__  ) 
/_____/_/   \____/|__/|__/\____/ .___/____/  
                              /_/            
    HTCPCP/1.0 Server (RFC 2324 + RFC 7168)
    Enterprise-Grade Beverage Infrastructure
    
    "Anyone who gets in between me and my 
     morning coffee should be insecure."
              — RFC 2324, Section 7

`

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8418" // A nod to HTTP 418
	}

	// Find web directory
	webDir := findWebDir()

	// Initialize the fleet, metrics collector, and HTCPCP server
	fleet := htcpcp.NewPotFleet()
	collector := metrics.NewCollector()
	htcpcpServer := htcpcp.NewServer(fleet, collector)
	htcpcpHandler := htcpcpServer.Handler()

	// Single root handler — no mux, no framework, just a function.
	// We route everything ourselves because Go's ServeMux doesn't
	// understand BREW, WHEN, or PROPFIND methods. Neither do most
	// humans, but here we are.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Dashboard
		if path == "/dashboard" || path == "/dashboard/" {
			http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
			return
		}

		// Static assets
		if path == "/style.css" || path == "/dashboard.js" {
			http.ServeFile(w, r, filepath.Join(webDir, filepath.Base(path)))
			return
		}

		// Browser hitting root? Redirect to dashboard.
		if path == "/" && r.Method == "GET" {
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "text/html") {
				http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
				return
			}
		}

		// Everything else: HTCPCP protocol server
		htcpcpHandler.ServeHTTP(w, r)
	})

	fmt.Print(banner)
	fmt.Printf("  Fleet Status:\n")
	for _, status := range fleet.AllStatus() {
		icon := "☕"
		if status.Type == htcpcp.PotTypeTeapot {
			icon = "🫖"
		}
		fmt.Printf("    %s pot-%d: %s (%s) — %s\n", icon, status.ID, status.Type, status.State, status.TempLabel)
	}
	fmt.Printf("\n  Dashboard: http://localhost:%s/dashboard\n\n", port)
	fmt.Printf("  Try these:\n")
	fmt.Printf("    curl -X BREW http://localhost:%s/pot -d 'start'               Brew coffee (auto-creates pot)\n", port)
	fmt.Printf("    curl -X BREW http://localhost:%s/pot-2 -d 'start'             418 I'm a Teapot!\n", port)
	fmt.Printf("    curl -X BREW http://localhost:%s/tea/earl-grey -d 'start'     Brew tea\n", port)
	fmt.Printf("    curl http://localhost:%s/status                               Fleet status\n\n", port)
	fmt.Printf("  Every 5th pot created is a surprise teapot.\n\n")
	fmt.Printf("  Listening on :%s...\n\n", port)

	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Failed to start BrewOps server: %v", err)
	}
}

func findWebDir() string {
	candidates := []string{
		"web",
		"./web",
		"/usr/share/brewops/web",
		filepath.Join(os.Getenv("HOME"), "brewops/web"),
	}

	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			absPath, _ := filepath.Abs(dir)
			return absPath
		}
	}

	return "."
}
