package setup

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Prompter asks the operator for values. Interactive mode reads stdin.
// Non-interactive mode (--non-interactive or no TTY) accepts values ONLY
// from explicit SETUP_<KEY> env vars or the given default; a required value
// with neither is recorded in Missing — the wizard aborts before applying
// anything, listing every gap at once. Ambient shell env vars never override
// a prompt silently (the SETUP_ prefix is the explicit opt-in).
type Prompter struct {
	NonInteractive bool
	Missing        []string
	in             *bufio.Reader
}

func NewPrompter(nonInteractive bool) *Prompter {
	return &Prompter{NonInteractive: nonInteractive, in: bufio.NewReader(os.Stdin)}
}

// setupEnv returns the explicit SETUP_<KEY> override, if any.
func setupEnv(key string) string { return os.Getenv("SETUP_" + key) }

// Ask prompts for a value. def=="" means required.
func (p *Prompter) Ask(key, label, def string) string {
	if v := setupEnv(key); v != "" {
		return v
	}
	if p.NonInteractive {
		if def == "" {
			p.Missing = append(p.Missing, "SETUP_"+key+" ("+label+")")
		}
		return def
	}
	for {
		if def != "" {
			fmt.Printf("%s [%s]: ", label, def)
		} else {
			fmt.Printf("%s: ", label)
		}
		line, _ := p.in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			line = def
		}
		if line != "" {
			return line
		}
		fmt.Println("  (majburiy qiymat / required)")
	}
}

// AskSecret prompts without echoing. def=="" means required.
func (p *Prompter) AskSecret(key, label, def string) string {
	if v := setupEnv(key); v != "" {
		return v
	}
	if p.NonInteractive {
		if def == "" {
			p.Missing = append(p.Missing, "SETUP_"+key+" ("+label+")")
		}
		return def
	}
	for {
		if def != "" {
			fmt.Printf("%s [saqlangan qiymat / keep existing]: ", label)
		} else {
			fmt.Printf("%s: ", label)
		}
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		v := sanitizeSecret(string(b))
		if err != nil { // not a terminal — fall back to plain read
			line, _ := p.in.ReadString('\n')
			v = sanitizeSecret(line)
		}
		if v == "" {
			v = def
		}
		if v != "" {
			return v
		}
		fmt.Println("  (majburiy qiymat / required)")
	}
}

// sanitizeSecret strips terminal escape sequences and control bytes that
// leak into raw reads — most notably bracketed-paste markers
// (ESC [ 200~ … ESC [ 201~) that zsh/tmux-era terminals wrap around pasted
// text. Without this, a PASTED password silently carries invisible bytes
// and every auth probe fails while the "same" password works in curl.
func sanitizeSecret(s string) string {
	var out []rune
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 0x1b { // ESC — CSI ketma-ketligini yutamiz (masalan [200~)
			for i+1 < len(rs) {
				i++
				c := rs[i]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '~' {
					break
				}
			}
			continue
		}
		if r < 0x20 || r == 0x7f { // control belgilarga ruxsat yo'q
			continue
		}
		out = append(out, r)
	}
	return strings.TrimSpace(string(out))
}

// AskYN prompts for yes/no.
func (p *Prompter) AskYN(key, label string, def bool) bool {
	if v := setupEnv(key); v != "" {
		return v == "y" || v == "yes" || v == "true" || v == "1"
	}
	if p.NonInteractive {
		return def
	}
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	fmt.Printf("%s [%s]: ", label, hint)
	line, _ := p.in.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return def
	}
	return line == "y" || line == "yes" || line == "д" || line == "ha"
}

// Choice is one option in AskChoice.
type Choice struct {
	Key   string // stable value, also what SETUP_<key> matches
	Label string
}

// AskChoice presents numbered options; returns the chosen Key.
// Non-interactive: SETUP_<key> must equal one of the Keys, else the default
// (first option) is used.
func (p *Prompter) AskChoice(key, label string, options []Choice) string {
	if v := setupEnv(key); v != "" {
		for _, o := range options {
			if o.Key == v {
				return v
			}
		}
		p.Missing = append(p.Missing, fmt.Sprintf("SETUP_%s=%q noma'lum variant (valid: %s)", key, v, choiceKeys(options)))
		return options[0].Key
	}
	if p.NonInteractive {
		return options[0].Key
	}
	fmt.Println(label)
	for i, o := range options {
		fmt.Printf("  %d) %s\n", i+1, o.Label)
	}
	for {
		fmt.Printf("Tanlang [1]: ")
		line, _ := p.in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return options[0].Key
		}
		for i, o := range options {
			if line == fmt.Sprint(i+1) || line == o.Key {
				return o.Key
			}
		}
		fmt.Println("  noto'g'ri tanlov")
	}
}

func choiceKeys(options []Choice) string {
	keys := make([]string, len(options))
	for i, o := range options {
		keys[i] = o.Key
	}
	return strings.Join(keys, "|")
}

// Pause waits for Enter (interactive only). Returns false if skipped.
func (p *Prompter) Pause(label string) bool {
	if p.NonInteractive {
		return false
	}
	fmt.Printf("%s [Enter=davom, s=skip]: ", label)
	line, _ := p.in.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(line)) != "s"
}
