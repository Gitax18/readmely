package ingest

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
)

// Finding is one secret-like value that was removed from a file. Never the value itself.
type Finding struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Rule string `json:"rule"`
}

type secretRule struct {
	name string
	re   *regexp.Regexp
	// group is the submatch to redact; 0 redacts the whole match.
	group int
}

// The rules favour catching too much over too little: a redacted example costs nothing, a leaked
// key to a model provider costs a lot.
var secretRules = []secretRule{
	{"private_key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z ]*PRIVATE KEY-----|\z)`), 0},
	{"aws_access_key", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), 0},
	{"github_token", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{30,255}|github_pat_[A-Za-z0-9_]{50,255})\b`), 0},
	{"slack_token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,72}\b`), 0},
	{"google_api_key", regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`), 0},
	{"stripe_key", regexp.MustCompile(`\b[sr]k_(?:live|test)_[0-9A-Za-z]{16,}\b`), 0},
	{"api_key", regexp.MustCompile(`\bsk-(?:proj-|ant-)?[A-Za-z0-9_\-]{24,}\b`), 0},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`), 0},
	{"credential_url", regexp.MustCompile(`\b[a-z][a-z0-9+.\-]*://[^\s:/@"']{1,64}:([^\s@/"']{6,})@`), 1},
	{"assigned_secret", regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|token|passw(?:or)?d|auth)[A-Za-z0-9_]*["']?\s*[:=]\s*["']([A-Za-z0-9/+_\-=.]{16,})["']`), 1},
}

var placeholderWords = []string{"example", "your", "xxxx", "changeme", "placeholder", "dummy", "sample", "test", "<", ">", "${", "{{", "todo", "redacted"}

func placeholderLike(v string) bool {
	lower := strings.ToLower(v)
	for _, w := range placeholderWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	// "aaaaaaaa", "00000000": not a secret.
	return len(strings.Trim(v, string(v[0]))) == 0
}

const redacted = "[REDACTED]"

// Scan removes secret-like values from a file's text and says where they were. The input is not
// changed. A file with nothing to remove is returned as is, without copying.
func Scan(filePath string, content []byte) ([]byte, []Finding) {
	type span struct{ from, to int }
	var spans []span
	var findings []Finding
	for _, rule := range secretRules {
		for _, m := range rule.re.FindAllSubmatchIndex(content, -1) {
			from, to := m[2*rule.group], m[2*rule.group+1]
			if from < 0 {
				continue
			}
			if rule.group > 0 && placeholderLike(string(content[from:to])) {
				continue
			}
			spans = append(spans, span{from, to})
			findings = append(findings, Finding{Path: filePath, Line: 1 + bytes.Count(content[:from], []byte{'\n'}), Rule: rule.name})
		}
	}
	if len(spans) == 0 {
		return content, nil
	}
	slices.SortFunc(spans, func(a, b span) int { return a.from - b.from })
	var out bytes.Buffer
	last := 0
	for _, s := range spans {
		if s.from < last { // overlaps a span already removed
			if s.to > last {
				last = s.to
			}
			continue
		}
		out.Write(content[last:s.from])
		out.WriteString(redacted)
		last = s.to
	}
	out.Write(content[last:])
	return out.Bytes(), findings
}
