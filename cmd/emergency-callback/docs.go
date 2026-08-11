package main

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	embedded "github.com/Diyarbekoralbaev/emergency_callback_go"
)

// runDocs serves the MkDocs site. Disk-first (DOCS_DIR yoki ./site — jonli
// mkdocs build uchun), aks holda release binariga embed qilingan nusxa.
// Ataylab config.Load() chaqirmaydi — hech qanday env talab qilmaydi.
// O'z portining ildizida beriladi (MkDocs absolyut URL'lar bilan quriladi).
func runDocs() {
	dir := os.Getenv("DOCS_DIR")
	if dir == "" {
		dir = "site"
	}
	addr := os.Getenv("DOCS_ADDR")
	if addr == "" {
		addr = ":8001"
	}

	var handler http.Handler
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		handler = http.FileServer(http.Dir(dir))
		slog.Info("docs: disk'dagi sayt", "dir", dir)
	} else if embedded.DocsEmbedded {
		sub, err := fs.Sub(embedded.Docs, "site")
		if err != nil {
			slog.Error("docs embed", "err", err)
			os.Exit(1)
		}
		handler = http.FileServer(http.FS(sub))
		slog.Info("docs: binardagi embed nusxa")
	} else {
		slog.Error("docs sayti topilmadi", "dir", dir,
			"hint", "release binar ishlating (docs embed qilingan) yoki mkdocs build qiling")
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", handler)

	slog.Info("docs server listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("docs listen", "err", err)
		os.Exit(1)
	}
}
