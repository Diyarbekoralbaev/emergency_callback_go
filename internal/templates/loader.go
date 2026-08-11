package templates

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Registry holds the parsed templates, keyed by their path relative to the
// templates root (e.g. "callbacks/list.html").
//
// Two template families:
//   - Layout templates extend base.html via `{{ define "content" }}` / `{{ define "title" }}`
//     and are rendered through Render(name, data).
//   - Standalone templates (anything matching "vote*" or login standalone)
//     have their own full HTML and are rendered through RenderStandalone(name, data).
type Registry struct {
	layout     map[string]*template.Template
	standalone map[string]*template.Template
	baseFile   string
}

// Load walks the given root directory and parses every .html file.
// base.html (at the root) is automatically included with every layout page.
// Files whose basename starts with "vote" or whose name is "login.html" are
// treated as standalone (no base).
func Load(root string) (*Registry, error) {
	return LoadFS(os.DirFS(root))
}

// LoadFS is Load over any fs.FS (a disk dir via os.DirFS, or the embedded
// copy). The FS root must contain base.html and the page files.
func LoadFS(fsys fs.FS) (*Registry, error) {
	reg := &Registry{
		layout:     map[string]*template.Template{},
		standalone: map[string]*template.Template{},
		baseFile:   "base.html",
	}
	if _, err := fs.Stat(fsys, reg.baseFile); err != nil {
		reg.baseFile = ""
	}

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".html") {
			return nil
		}
		rel := p // fs paths are already slash-separated and root-relative

		// Skip the base file itself (already loaded with every layout page)
		if rel == "base.html" {
			return nil
		}

		standalone := strings.HasPrefix(path.Base(p), "vote")

		t := template.New(path.Base(p)).Funcs(FuncMap())

		if standalone || reg.baseFile == "" {
			parsed, err := t.ParseFS(fsys, p)
			if err != nil {
				return fmt.Errorf("parse %s: %w", rel, err)
			}
			reg.standalone[rel] = parsed
		} else {
			parsed, err := t.ParseFS(fsys, reg.baseFile, p)
			if err != nil {
				return fmt.Errorf("parse %s: %w", rel, err)
			}
			reg.layout[rel] = parsed
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reg, nil
}

// Render writes a layout (base.html-extending) template.
func (r *Registry) Render(w io.Writer, name string, data any) error {
	t, ok := r.layout[name]
	if !ok {
		// Fallback: maybe it's standalone
		t, ok = r.standalone[name]
		if !ok {
			return fmt.Errorf("template %s not found", name)
		}
		return t.ExecuteTemplate(w, filepath.Base(name), data)
	}
	return t.ExecuteTemplate(w, "base", data)
}

// RenderStandalone writes a self-contained (non-base) template, e.g. vote pages.
func (r *Registry) RenderStandalone(w io.Writer, name string, data any) error {
	t, ok := r.standalone[name]
	if !ok {
		return fmt.Errorf("standalone template %s not found", name)
	}
	return t.ExecuteTemplate(w, filepath.Base(name), data)
}

// Names returns all registered template names — useful for startup logging.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.layout)+len(r.standalone))
	for k := range r.layout {
		out = append(out, k+" (layout)")
	}
	for k := range r.standalone {
		out = append(out, k+" (standalone)")
	}
	return out
}
