package setup

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// EnvFile is an order- and comment-preserving .env editor. Setup merges new
// values into an existing file instead of regenerating it, so manual edits,
// unknown keys and secrets survive re-runs.
type EnvFile struct {
	path     string
	lines    []string          // raw lines, in order
	index    map[string]int    // key -> line number
	original map[string]string // values as loaded (for Diff)
}

// LoadEnvFile parses path; a missing file yields an empty, writable EnvFile.
func LoadEnvFile(path string) (*EnvFile, error) {
	f := &EnvFile{
		path:     path,
		index:    map[string]int{},
		original: map[string]string{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	f.lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	for i, line := range f.lines {
		k, v, ok := parseEnvLine(line)
		if ok {
			f.index[k] = i
			f.original[k] = v
		}
	}
	return f, nil
}

func parseEnvLine(line string) (key, val string, ok bool) {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", "", false
	}
	eq := strings.Index(s, "=")
	if eq <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:eq]), strings.TrimSpace(s[eq+1:]), true
}

// Get returns the current value of key ("" if absent).
func (f *EnvFile) Get(key string) string {
	if i, ok := f.index[key]; ok {
		_, v, _ := parseEnvLine(f.lines[i])
		return v
	}
	return ""
}

// Has reports whether key exists with a non-empty value.
func (f *EnvFile) Has(key string) bool { return f.Get(key) != "" }

// Set updates an existing line in place or appends a new one.
func (f *EnvFile) Set(key, value string) {
	line := key + "=" + value
	if i, ok := f.index[key]; ok {
		f.lines[i] = line
		return
	}
	f.lines = append(f.lines, line)
	f.index[key] = len(f.lines) - 1
}

// Diff lists keys whose value changed vs. what was loaded (new keys included).
func (f *EnvFile) Diff() []string {
	var out []string
	for k, i := range f.index {
		_, v, _ := parseEnvLine(f.lines[i])
		if old, existed := f.original[k]; !existed || old != v {
			out = append(out, k)
		}
	}
	return out
}

// Save writes the file with mode 0600, backing up an existing file first.
func (f *EnvFile) Save() error {
	if _, err := os.Stat(f.path); err == nil {
		backup := fmt.Sprintf("%s.bak.%d", f.path, time.Now().Unix())
		if data, err := os.ReadFile(f.path); err == nil {
			_ = os.WriteFile(backup, data, 0o600)
		}
	}
	content := strings.Join(f.lines, "\n") + "\n"
	return os.WriteFile(f.path, []byte(content), 0o600)
}
