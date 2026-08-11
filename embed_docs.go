//go:build embeddocs

package embedded

import "embed"

// Docs is the pre-built MkDocs site, baked in ONLY for release builds
// (go build -tags embeddocs, after `mkdocs build`). Dev builds use the
// stub in embed_docs_stub.go so the repo compiles without Python.
//
//go:embed all:site
var Docs embed.FS

// DocsEmbedded reports whether this binary carries the docs site.
const DocsEmbedded = true
