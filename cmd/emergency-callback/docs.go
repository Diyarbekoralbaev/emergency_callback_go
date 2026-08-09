package main

import (
	"log/slog"
	"net/http"
	"os"
)

// runDocs serves the pre-built MkDocs site (the `site/` directory) as static
// files on its own port. Deliberately standalone: it does NOT call
// config.Load(), so the docs server needs none of the app's env vars
// (DATABASE_URL, AMI_*, …) — it only reads DOCS_DIR and DOCS_ADDR.
//
// Served at the root of its own port (not a subpath), because MkDocs builds
// for root and subpath hosting breaks its absolute asset URLs.
func runDocs() {
	dir := os.Getenv("DOCS_DIR")
	if dir == "" {
		dir = "site"
	}
	addr := os.Getenv("DOCS_ADDR")
	if addr == "" {
		addr = ":8001"
	}

	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		slog.Error("docs dir not found", "dir", dir, "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(dir)))

	slog.Info("docs server listening", "addr", addr, "dir", dir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("docs listen", "err", err)
		os.Exit(1)
	}
}
