package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	listPageSize = 100
	// listMaxPages bounds the list: 300 repositories is more than the screen is meant to show.
	listMaxPages = 3
	// A page of 100 full repository objects is far larger than one repository.
	maxListResponse = 4 << 20
)

// ListRepositories returns the repositories the person's token can see (their own, those of
// organisations they belong to, and ones they collaborate on), most recently pushed first. Only a
// client made with WithToken may call it. Archived and disabled repositories are left out: there is
// nothing to document in a repository that is frozen or switched off.
func (c *Client) ListRepositories(ctx context.Context) ([]Repository, error) {
	if !c.asUser {
		return nil, ErrUnauthorized
	}
	var out []Repository
	for page := 1; page <= listMaxPages; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(
			"%s/user/repos?per_page=%d&page=%d&sort=pushed&affiliation=owner,collaborator,organization_member",
			c.base, listPageSize, page), nil)
		if err != nil {
			return nil, ErrUnavailable
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", "readmely")
		req.Header.Set("Authorization", "Bearer "+c.token)
		res, err := c.http.Do(req)
		if err != nil {
			return nil, ErrUnavailable
		}
		var rows []repoBody
		status := res.StatusCode
		if status == http.StatusOK {
			if err := json.NewDecoder(io.LimitReader(res.Body, maxListResponse)).Decode(&rows); err != nil {
				status = http.StatusBadGateway
			}
		}
		_ = res.Body.Close()
		switch {
		case status == http.StatusUnauthorized:
			return nil, ErrUnauthorized
		case status != http.StatusOK:
			return nil, ErrUnavailable
		}
		for _, row := range rows {
			if row.Archived || row.Disabled {
				continue
			}
			if repo, ok := row.repository(); ok {
				out = append(out, repo)
			}
		}
		if len(rows) < listPageSize {
			break
		}
	}
	return out, nil
}
