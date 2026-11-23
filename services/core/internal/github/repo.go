// Package github reads public repository details from GitHub's REST API. It talks to one fixed
// host, follows no redirects, and keeps every response small, like the sign-in provider clients.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	apiBase     = "https://api.github.com"
	httpTimeout = 5 * time.Second
	maxResponse = 256 << 10
)

// Errors the project service turns into answers.
var (
	ErrInvalidURL  = errors.New("not a github.com repository address")
	ErrNotFound    = errors.New("repository not found or not public")
	ErrUnavailable = errors.New("github is not answering")
)

var (
	ownerRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
	nameRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	// A branch name as stored: letters, digits and . _ / - only. Anything else is refused rather than kept.
	branchRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,255}$`)
)

// Ref names a repository.
type Ref struct{ Owner, Name string }

func (r Ref) String() string { return r.Owner + "/" + r.Name }

// ParseURL accepts what people paste: https://github.com/owner/repo, with or without the scheme,
// "www.", a trailing slash or ".git", or extra path such as /tree/main. Anything else is refused.
func ParseURL(raw string) (Ref, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 300 || strings.ContainsAny(raw, "\x00\r\n\t ") {
		return Ref{}, ErrInvalidURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return Ref{}, ErrInvalidURL
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return Ref{}, ErrInvalidURL
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return Ref{}, ErrInvalidURL
	}
	owner, name := parts[0], strings.TrimSuffix(parts[1], ".git")
	if !ownerRe.MatchString(owner) || !nameRe.MatchString(name) || name == "." || name == ".." {
		return Ref{}, ErrInvalidURL
	}
	return Ref{Owner: owner, Name: name}, nil
}

// Repository is what version 1 shows and stores about a repository.
type Repository struct {
	Ref
	Description   string
	DefaultBranch string
	SizeKB        int
}

// Client reads from the GitHub API. The token is optional: it only raises the rate limit.
type Client struct {
	http    *http.Client
	archive *http.Client // downloads; its time limit is the caller's context
	base    string
	token   string
}

// NewClient returns a client for api.github.com.
func NewClient(token string) *Client {
	return &Client{
		http: &http.Client{
			Timeout:       httpTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		archive: &http.Client{CheckRedirect: archiveRedirect},
		base:    apiBase,
		token:   token,
	}
}

// Repository returns the public repository, or ErrNotFound when it does not exist or is private
// (GitHub answers both the same way to a request without access).
func (c *Client) Repository(ctx context.Context, ref Ref) (Repository, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/repos/"+url.PathEscape(ref.Owner)+"/"+url.PathEscape(ref.Name), nil)
	if err != nil {
		return Repository{}, fmt.Errorf("github: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "readmely")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Repository{}, ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone || res.StatusCode == http.StatusUnavailableForLegalReasons:
		return Repository{}, ErrNotFound
	case res.StatusCode != http.StatusOK:
		// Includes rate limiting (403/429) and server errors.
		return Repository{}, ErrUnavailable
	}
	var body struct {
		Private       bool                   `json:"private"`
		FullName      string                 `json:"full_name"`
		Name          string                 `json:"name"`
		Owner         struct{ Login string } `json:"owner"`
		Description   *string                `json:"description"`
		DefaultBranch string                 `json:"default_branch"`
		Size          int                    `json:"size"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponse)).Decode(&body); err != nil {
		return Repository{}, ErrUnavailable
	}
	if body.Private {
		return Repository{}, ErrNotFound
	}
	// Use what GitHub says the repository is called (case, renames), but only if it is well formed.
	if !ownerRe.MatchString(body.Owner.Login) || !nameRe.MatchString(body.Name) || !safeBranch(body.DefaultBranch) || body.Size < 0 {
		return Repository{}, ErrUnavailable
	}
	desc := ""
	if body.Description != nil {
		desc = cleanDescription(*body.Description)
	}
	return Repository{
		Ref:           Ref{Owner: body.Owner.Login, Name: body.Name},
		Description:   desc,
		DefaultBranch: body.DefaultBranch,
		SizeKB:        body.Size,
	}, nil
}

// cleanDescription drops control characters and caps the length, so a description can be shown
// and stored without surprises. React escapes it on screen as well; this is the second layer.
func cleanDescription(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 500 {
		s = string(r[:500])
	}
	return s
}

// safeBranch accepts the characters a branch name is stored with and nothing that climbs a path.
func safeBranch(b string) bool {
	return branchRe.MatchString(b) && !strings.Contains(b, "..") && !strings.HasPrefix(b, "/") && !strings.HasSuffix(b, "/")
}
