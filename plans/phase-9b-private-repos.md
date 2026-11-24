# Phase 9b · Private GitHub repositories

**Status:** Built, awaiting review · **Last updated:** 2026-11-24 · Branch `feat/private-repos` (stacked on `feat/ui-redesign`) · Parent plan: [v1-roadmap.md](v1-roadmap.md)

A person who connected GitHub can pick one of their **private** repositories on the new-project screen. Until now projects were public repositories only, read with one server-wide token, and no provider token was ever stored.

## 1. Decisions

| Question | Decision | Why |
|---|---|---|
| How is access granted | GitHub OAuth, scope `repo`, asked in a separate, explicit step, not at sign-in | Sign-in keeps its small scope. The `repo` scope is read **and write** for every private repository; GitHub's OAuth Apps have no read-only private scope. The screen says so plainly. A GitHub App (read-only, per repository) is the better long-term design and is listed under risks |
| Where the token lives | Table `github_access`, one row per user, sealed with AES-256-GCM; the user id is bound in as additional data | A database copy alone cannot read it; a row cannot be moved to another user |
| The key | `GITHUB_TOKEN_KEY` (32 bytes, hex). The feature is off when it is unset | Same shape as `AUTH_CODE_KEY` |
| What the token is used for | Listing the person's repositories, and reading the repository for **their** projects. Nothing else, never logged, never sent to the browser or the AI service | |
| Revoking | "Remove access" on the account page deletes the row; disconnecting GitHub, deleting the account, and a 401 from GitHub do the same | |
| Private projects | `projects.is_private` is set when the repository was read with the person's token. The project shows a "Private" badge | Lets the UI and the privacy page say what is stored |

## 2. Changes

| Layer | Change |
|---|---|
| Database | Migration: `github_access`, `projects.is_private` |
| Go | `secretbox` (seal and open), `githubaccess` (store), GitHub client with a per-user token and a repository list, OAuth intent `repos`, `GET/DELETE /v1/me/github/access`, `POST /v1/me/github/access` (start), `GET /v1/me/github/repos`; projects use the token when a repository is not public; the ingest worker opens the owner's token |
| Contract | `contracts/core-public.yaml`, regenerated Go and TypeScript |
| Web | New-project screen lists the person's repositories (search, Private badge, Add); account page shows the access state with Allow and Remove; project header shows Private |
| Docs | Privacy page states that private code is stored and sent for analysis the same way as public code |

## 3. Verification

No new tests (fast-forward rule); the existing Go and web suites must stay green. Checked by hand against a fake GitHub.

## 4. Risks

| Risk | Plan |
|---|---|
| `repo` scope is write access | Said on the consent screen and the account page; Readmely only ever issues GET requests; a read-only GitHub App is the follow-up |
| A stored token is a high-value secret | Sealed, key outside the database, bound to the user, deleted on every exit path, never logged |
| Private code reaches the AI service | It already receives public code the same way; the privacy page now says private repositories are included |
