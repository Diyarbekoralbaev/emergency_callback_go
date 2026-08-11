package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The merge behavior is the wizard's core data-safety property: re-running
// setup must never lose manual edits, comments or secrets.
func TestEnvFileMergePreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	orig := `# Database
DATABASE_URL=postgres://old
SESSION_SECRET=keepme

# custom manual tuning
MY_CUSTOM_FLAG=1
`
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := LoadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Get("SESSION_SECRET"); got != "keepme" {
		t.Fatalf("Get = %q", got)
	}

	f.Set("DATABASE_URL", "postgres://new") // update in place
	f.Set("HTTP_ADDR", ":9000")             // append new

	diff := f.Diff()
	if len(diff) != 2 {
		t.Fatalf("Diff = %v, want 2 entries", diff)
	}

	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	out := string(data)

	for _, want := range []string{
		"# Database",                 // comment preserved
		"# custom manual tuning",     // comment preserved
		"MY_CUSTOM_FLAG=1",           // unknown key preserved
		"SESSION_SECRET=keepme",      // untouched secret preserved
		"DATABASE_URL=postgres://new", // updated in place
		"HTTP_ADDR=:9000",            // appended
	} {
		if !strings.Contains(out, want) {
			t.Errorf("saved .env missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "postgres://old") {
		t.Error("old value still present")
	}
	// order: DATABASE_URL must still be on its original line (2nd)
	lines := strings.Split(out, "\n")
	if lines[1] != "DATABASE_URL=postgres://new" {
		t.Errorf("line order broken: %q", lines[1])
	}

	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", st.Mode().Perm())
	}
}

func TestEnvFileMissingFile(t *testing.T) {
	f, err := LoadEnvFile(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("A", "1")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if f.Get("A") != "1" {
		t.Fatal("roundtrip failed")
	}
}
