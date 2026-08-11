package setup

import "testing"

// Pasted secrets arrive wrapped in bracketed-paste markers on modern
// terminals; typed ones may carry stray \r. Both must come out clean.
func TestSanitizeSecret(t *testing.T) {
	cases := map[string]string{
		"labari":                         "labari",
		"labari\r":                       "labari",
		"\x1b[200~labari\x1b[201~":       "labari", // bracketed paste
		"\x1b[200~labari\x1b[201~\r":     "labari",
		"  labari  ":                     "labari",
		"la\x00bari":                     "labari", // control byte
		"p@ss w0rd!":                     "p@ss w0rd!",
		"\x1b[0mclean":                   "clean", // SGR sequence
	}
	for in, want := range cases {
		if got := sanitizeSecret(in); got != want {
			t.Errorf("sanitizeSecret(%q) = %q, want %q", in, got, want)
		}
	}
}
