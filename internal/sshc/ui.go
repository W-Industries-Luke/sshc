package sshc

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

// ui collects what one command has to tell the user and prints it as a
// block: a blank line above and below, and a blank line between messages.
// On a terminal the key words are coloured; piped or redirected output, and
// output under NO_COLOR or TERM=dumb, stays plain.
type ui struct {
	w     *os.File
	color bool
	msgs  []string
}

// lastBlank records that the previous block already ended in a blank line, so
// that two blocks in a row are not separated by two.
var lastBlank bool

func newUI(w *os.File) *ui {
	color := term.IsTerminal(int(w.Fd())) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return &ui{w: w, color: color}
}

// say adds one message; it may span several lines.
func (u *ui) say(format string, a ...any) {
	u.msgs = append(u.msgs, fmt.Sprintf(format, a...))
}

// flush prints the collected messages, if any.
func (u *ui) flush() {
	if len(u.msgs) == 0 {
		return
	}
	text := strings.Join(u.msgs, "\n\n")
	if u.color {
		enableVT(u.w)
		text = highlight(text)
	}
	lead := "\n"
	if lastBlank {
		lead = ""
	}
	fmt.Fprint(u.w, lead+text+"\n\n")
	lastBlank = true
	u.msgs = nil
}

const (
	sgrReset  = "\x1b[0m"
	sgrBold   = "\x1b[1m"
	sgrGreen  = "\x1b[1;32m"
	sgrYellow = "\x1b[33m"
	sgrRed    = "\x1b[1;31m"
	sgrCyan   = "\x1b[36m"
)

// The words worth picking out, in the order they are applied. Each pattern
// colours its first group, or the whole match if it has none.
var highlights = []struct {
	re    *regexp.Regexp
	color string
}{
	// Outcomes.
	{regexp.MustCompile(`(?m)^(Updated!|Removed!|Unlocked!|Installed|Created|Added|Moved|Kept|Done:)`), sgrGreen},
	{regexp.MustCompile(`(?m)^(Locked!|LOCKED:)`), sgrRed},
	{regexp.MustCompile(`(?m)^(sshc:)`), sgrRed},
	{regexp.MustCompile(`(?m)^\s*(Warning:)`), sgrRed},
	{regexp.MustCompile(`(?m)^\s*(Note:|Optional:)`), sgrYellow},
	{regexp.MustCompile(`MISSING|was not accepted|not available|not readable here`), sgrRed},
	{regexp.MustCompile(`(?m)^(config file|credential store|active profile|Stored|On login|Set in this terminal|Hosts in [^:\n]*)\b[^:\n]*:`), sgrBold},
	{regexp.MustCompile(`no credential store is available here`), sgrYellow},
	// Where a secret is.
	{regexp.MustCompile(`Windows Credential Manager|the macOS Keychain|the system keyring \(Secret Service\)|credential store|for this terminal session only`), sgrGreen},
	{regexp.MustCompile(`plain text|no stored pass(?:word|phrase)|no usable passphrase|Nothing is stored\.|you would be prompted`), sgrYellow},
	// Names of things.
	{regexp.MustCompile(`\[(?:profile|host|key) [^\]\n]+\]`), sgrCyan},
	{regexp.MustCompile(`\bSSHC_[A-Z0-9_]+\b`), sgrCyan},
	// Commands the user is told to run.
	{regexp.MustCompile(`"(sshc [^"\n]*)"`), sgrBold},
}

// highlight colours the key words of a finished message.
func highlight(text string) string {
	// Colour codes are inserted from the last match backwards, and a span
	// that has been coloured once is left alone, so patterns cannot nest.
	type span struct {
		start, end int
		color      string
	}
	var spans []span
	taken := func(s, e int) bool {
		for _, sp := range spans {
			if s < sp.end && sp.start < e {
				return true
			}
		}
		return false
	}
	for _, h := range highlights {
		for _, m := range h.re.FindAllStringSubmatchIndex(text, -1) {
			s, e := m[0], m[1]
			if len(m) >= 4 && m[2] >= 0 {
				s, e = m[2], m[3]
			}
			if !taken(s, e) {
				spans = append(spans, span{s, e, h.color})
			}
		}
	}
	// Apply right to left so earlier offsets stay valid.
	for len(spans) > 0 {
		last := 0
		for i, sp := range spans {
			if sp.start > spans[last].start {
				last = i
			}
		}
		sp := spans[last]
		text = text[:sp.start] + sp.color + text[sp.start:sp.end] + sgrReset + text[sp.end:]
		spans = append(spans[:last], spans[last+1:]...)
	}
	return text
}
