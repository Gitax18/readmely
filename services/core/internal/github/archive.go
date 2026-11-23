package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	// MaxArchiveBytes is the most compressed data read from one archive.
	MaxArchiveBytes = 150 << 20
	codeloadHost    = "codeload.github.com"
)

// ErrArchiveTooLarge is returned when an archive is bigger than MaxArchiveBytes.
var ErrArchiveTooLarge = errors.New("archive is too large")

var shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// archiveRedirect lets the archive request follow GitHub's one redirect to codeload and nothing else.
func archiveRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 1 || req.URL.Scheme != "https" || req.URL.Hostname() != codeloadHost || req.URL.Port() != "" {
		return http.ErrUseLastResponse
	}
	return nil
}

func (c *Client) get(ctx context.Context, hc *http.Client, path, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "readmely")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	return res, nil
}

// CommitSHA returns the commit a branch points to now, so the whole analysis reads one snapshot.
func (c *Client) CommitSHA(ctx context.Context, ref Ref, branch string) (string, error) {
	if !safeBranch(branch) {
		return "", ErrNotFound
	}
	path := "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Name) + "/commits/" + branch
	res, err := c.get(ctx, c.http, path, "application/vnd.github.sha")
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusUnprocessableEntity:
		return "", ErrNotFound
	case res.StatusCode != http.StatusOK:
		return "", ErrUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 128))
	sha := strings.TrimSpace(string(b))
	if err != nil || !shaRe.MatchString(sha) {
		return "", ErrUnavailable
	}
	return sha, nil
}

// Archive streams the repository at a commit as a gzipped tarball. The caller must close it and
// must treat every entry as untrusted. Reading past MaxArchiveBytes fails with ErrArchiveTooLarge.
func (c *Client) Archive(ctx context.Context, ref Ref, sha string) (io.ReadCloser, error) {
	if !shaRe.MatchString(sha) {
		return nil, ErrNotFound
	}
	path := "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Name) + "/tarball/" + sha
	res, err := c.get(ctx, c.archive, path, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		if res.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, ErrUnavailable
	}
	return &limitedBody{rc: res.Body, left: MaxArchiveBytes}, nil
}

type limitedBody struct {
	rc   io.ReadCloser
	left int64
}

func (l *limitedBody) Read(p []byte) (int, error) {
	if l.left <= 0 {
		return 0, ErrArchiveTooLarge
	}
	if int64(len(p)) > l.left {
		p = p[:l.left]
	}
	n, err := l.rc.Read(p)
	l.left -= int64(n)
	return n, err
}

func (l *limitedBody) Close() error { return l.rc.Close() }
