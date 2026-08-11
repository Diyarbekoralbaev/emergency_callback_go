//go:build !embeddocs

package embedded

import "embed"

// Docs is empty in dev builds — the `docs` subcommand then requires an
// on-disk site/ (mkdocs build). Release builds use embed_docs.go.
var Docs embed.FS

const DocsEmbedded = false
