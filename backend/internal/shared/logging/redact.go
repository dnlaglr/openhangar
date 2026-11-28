package logging

import (
	"io"
	"os"
	"regexp"
	"slices"
)

type rule struct {
	re   *regexp.Regexp
	repl []byte
}

func newRule(pattern, repl string) rule {
	return rule{regexp.MustCompile(pattern), []byte(repl)}
}

// URLs go first: masking a token inside a URL earlier would cut the URL match short and leak its host and path.
// TODO: mask bare Tower hostnames in net errors ("lookup tower.example.com") when sync lands.
var baseRules = []rule{
	newRule(`(?i)\b([a-z][a-z0-9+.-]*://)[^\s"'<>\\]+`, `${1}<redacted>`),
	newRule(`(?i)("[\w-]*(?:token|secret|passw(?:or)?d|credential|api[_-]?key|authorization|cookie)[\w-]*":)"(?:[^"\\]|\\.)*"`, `${1}"<redacted>"`),
	newRule(`(?i)\b([\w-]*(?:token|secret|passw(?:or)?d|credential|api[_-]?key)[\w-]*=)[^\s&"\\]+`, `${1}<redacted>`),
	newRule(`(?i)(authorization[:=]\s*(?:(?:bearer|basic|token)\s+)?)[^\s"\\]+`, `${1}<redacted>`),
	newRule(`(?i)\b(bearer\s+)[\w.~+/=-]{8,}`, `${1}<redacted>`),
	newRule(`\b(?:sk-ant-[\w-]{8,}|sk-[\w-]{20,}|gh[pousr]_\w{30,}|github_pat_\w{20,}|glpat-[\w-]{20,}|xox[abprs]-[\w-]{10,}|AKIA[0-9A-Z]{16}|AIza[\w-]{35}|eyJ[\w-]{8,}\.[\w-]{8,}\.[\w-]{8,})`, `<redacted>`),
	newRule(`[\w.-]+@[\w.-]+:[\w./~-]+`, `<redacted>`),
	newRule(`[\w.+-]+@[\w-]+(?:\.[\w-]+)+`, `<redacted>`),
	newRule(`(?i)(?:/mnt/[a-z]|\b[a-z]:)(?:/|\\{1,2})users(?:/|\\{1,2})[^/\\"\s]+`, `<winhome>`),
}

var branchRule = newRule(`(^|[^/\w.]|heads/)hangar/[\w.-]+`, `${1}hangar/<redacted>`)

type Redactor struct {
	rules []rule
}

func NewRedactor(home, hostname string) *Redactor {
	rules := slices.Clone(baseRules)
	if home != "" && home != "/" {
		rules = append(rules, newRule(regexp.QuoteMeta(home)+`([/"\\\s:]|$)`, `~${1}`))
	}
	if len(hostname) >= 4 {
		rules = append(rules, newRule(`\b`+regexp.QuoteMeta(hostname)+`\b`, `<host>`))
	}

	return &Redactor{rules: append(rules, branchRule)}
}

func SystemRedactor() *Redactor {
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()

	return NewRedactor(home, host)
}

func (r *Redactor) Redact(b []byte) []byte {
	for _, rl := range r.rules {
		b = rl.re.ReplaceAll(b, rl.repl)
	}

	return b
}

func (r *Redactor) Writer(w io.Writer) io.Writer {
	return redactWriter{w: w, r: r}
}

type redactWriter struct {
	w io.Writer
	r *Redactor
}

func (rw redactWriter) Write(p []byte) (int, error) {
	if _, err := rw.w.Write(rw.r.Redact(p)); err != nil {
		return 0, err
	}

	return len(p), nil
}
