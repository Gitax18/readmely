// Package readme holds a project's README: its versions, its health score, and the requests that
// have the AI worker write or rewrite it.
package readme

import (
	"regexp"
	"strings"
)

// ScoreItem is one line of the health checklist.
type ScoreItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	OK    bool   `json:"ok"`
	Hint  string `json:"hint"`
}

// Score is the checklist result: points for what is there, and what is missing.
type Score struct {
	Score int         `json:"score"`
	Items []ScoreItem `json:"items"`
}

var (
	reInstall = regexp.MustCompile(`(?i)install|getting started|setup|set up|quick ?start`)
	reUsage   = regexp.MustCompile(`(?i)usage|example|how to use|running|run it`)
	reConfig  = regexp.MustCompile(`(?i)config|environment|settings|options`)
	reStruct  = regexp.MustCompile(`(?i)structure|layout|architecture|how it works|overview of the code`)
	reLicense = regexp.MustCompile(`(?i)licen[cs]e`)
	reContrib = regexp.MustCompile(`(?i)contribut`)
	reBadge   = regexp.MustCompile(`\[!\[|!\[[^\]]*\]\(https?://[^)]*(shields\.io|badge|badgen)`)
	reTitle   = regexp.MustCompile(`(?m)^# \S`)
)

type check struct {
	key, label, hint string
	points           int
	ok               func(content string, headings []string) bool
}

func anyHeading(headings []string, re *regexp.Regexp) bool {
	for _, h := range headings {
		if re.MatchString(h) {
			return true
		}
	}
	return false
}

var checks = []check{
	{"title", "A title", "Start with a # heading that names the project.", 10,
		func(c string, _ []string) bool { return reTitle.MatchString(c) }},
	{"description", "A description", "Say in a sentence or two what the project is, right under the title.", 10,
		func(c string, _ []string) bool { return hasIntro(c) }},
	{"install", "How to install or set up", "Add an Installation or Getting started section.", 15,
		func(_ string, h []string) bool { return anyHeading(h, reInstall) }},
	{"usage", "How to use it", "Add a Usage section with an example.", 15,
		func(_ string, h []string) bool { return anyHeading(h, reUsage) }},
	{"code", "Code examples", "Show commands or code in fenced blocks.", 10,
		func(c string, _ []string) bool { return strings.Count(c, "```") >= 2 }},
	{"configuration", "Configuration", "Explain the settings or environment variables it reads.", 10,
		func(_ string, h []string) bool { return anyHeading(h, reConfig) }},
	{"structure", "How the code is organised", "Add a Project structure or Architecture section.", 10,
		func(_ string, h []string) bool { return anyHeading(h, reStruct) }},
	{"license", "A licence", "Name the licence.", 10,
		func(_ string, h []string) bool { return anyHeading(h, reLicense) }},
	{"contributing", "How to contribute", "Add a Contributing section.", 5,
		func(_ string, h []string) bool { return anyHeading(h, reContrib) }},
	{"badges", "Badges", "Add status badges (build, licence, version).", 5,
		func(c string, _ []string) bool { return reBadge.MatchString(c) }},
}

// hasIntro is true when a paragraph of prose follows the title before the first section.
func hasIntro(content string) bool {
	lines := strings.Split(content, "\n")
	seenTitle := false
	words := 0
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "# "):
			seenTitle = true
		case strings.HasPrefix(t, "## "):
			return words >= 3
		case seenTitle && t != "" && !strings.HasPrefix(t, "[![") && !strings.HasPrefix(t, "!["):
			words += len(strings.Fields(t))
		}
	}
	return words >= 3
}

// ScoreOf grades a README against the checklist. It is deterministic and judges structure only: it
// says what is there, not whether it is any good.
func ScoreOf(content string) Score {
	heads := Headings(content)
	titles := make([]string, 0, len(heads))
	for _, h := range heads {
		titles = append(titles, h.Title)
	}
	out := Score{Items: make([]ScoreItem, 0, len(checks))}
	for _, c := range checks {
		ok := c.ok(content, titles)
		if ok {
			out.Score += c.points
		}
		out.Items = append(out.Items, ScoreItem{Key: c.key, Label: c.label, OK: ok, Hint: c.hint})
	}
	return out
}
