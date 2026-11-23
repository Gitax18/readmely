package readme

import "strings"

// Heading is one "## Title" line and where its section starts and ends (line indexes).
type Heading struct {
	Title      string
	Start, End int // lines [Start, End): the heading line through the line before the next heading
}

// Headings lists the level-2 headings of a markdown document, ignoring any inside fenced code.
func Headings(content string) []Heading {
	lines := strings.Split(content, "\n")
	var out []Heading
	fenced := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || !strings.HasPrefix(l, "## ") {
			continue
		}
		if n := len(out); n > 0 {
			out[n-1].End = i
		}
		out = append(out, Heading{Title: strings.TrimSpace(strings.TrimPrefix(l, "## ")), Start: i})
	}
	if n := len(out); n > 0 {
		out[n-1].End = len(lines)
	}
	return out
}

// HasHeading reports whether the document has a level-2 heading with this exact title.
func HasHeading(content, title string) bool {
	for _, h := range Headings(content) {
		if h.Title == title {
			return true
		}
	}
	return false
}
