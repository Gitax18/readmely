# Version 1 brief: what you can expect

**Status:** Approved (2026-11-24) · **Last updated:** 2026-11-24 · Build order and scope rules: [v1-roadmap.md](v1-roadmap.md)

## 1. The product in one paragraph

Readmely reads a public GitHub repository and produces three things from what it understands: a **README**, a **context pack** that helps AI coding tools work in the repository, and an **architecture diagram**. You paste a repository URL, watch it being analysed, then generate, edit and download each output. Everything is paid for with credits; every new account gets a free grant.

## 2. Who it is for

- A developer who has a repository with a weak or missing README and wants a good one in minutes.
- A developer who uses Claude Code, Cursor or similar tools and wants a ready-made `CLAUDE.md` or `AGENTS.md`.
- Someone who needs a clean architecture picture for a README, a portfolio or an interview.

## 3. Features

| Area | What version 1 does |
|---|---|
| **Sign-in** (built) | Emailed one-time codes, Google, GitHub; sessions across devices; connect and disconnect providers; sign-in alerts by email |
| **Account** | Rename, list and end sessions, sign out other devices, delete the account and all its data |
| **Projects** | Create from a public GitHub URL, list, open, delete; at most 10 per user; re-analyse to pick up new commits |
| **Analysis** | Languages and line counts, frameworks and libraries, entry points, run and test commands, services (from Docker and compose files), environment variables, license, file tree, a plain-English summary of what the project does. Secrets found in the code are never stored or shown; you see a count and file locations |
| **README** | Three templates (Standard, Minimal, Detailed); the sections are chosen to fit the project type; edit in a Markdown editor with live preview; regenerate one section with an instruction; version history and restore; a health score with a list of what is missing; copy or download |
| **Context pack** | Presets for `CLAUDE.md`, `AGENTS.md`, Cursor rules and a generic `CONTEXT.md`; a token estimate for each; preview, copy, download as a ZIP |
| **Diagram** | An overview architecture diagram of 5 to 9 components with coloured categories, zoom and pan, regenerate with a focus note, download as SVG or PNG, copy the Markdown to embed it in the README |
| **Credits** | 100 free credits at sign-up; a balance in the top bar; a usage page with history; clear messages when credits run out |
| **Site** | Landing page with a real sample, privacy and terms pages, light and dark themes, phone-friendly layouts, accessible forms |

### Credit costs

Repriced on 2026-11-25: every service costs 1.75 times its model cost and a diagram 2.5 times; the grant stays at 100. Each job holds its price up front and is charged what its model calls cost at that rate, at least one credit before the multiplier; the rest of the hold comes back.

| Action | Held up front | Usually charged |
|---|---|---|
| Analyse a project (includes the summary) | 18 | about 2 |
| Context pack | free | free (no model call) |
| README (full generation) | 27 | 2 to 4 |
| Regenerate one README section | 4 | 2 |
| Architecture or database diagram | 38 | about 3 |

Holding everything for one project at once would take 83 credits, but jobs run one at a time and return what they don't use, so a project with all its outputs is usually charged about 11. Failed jobs are refunded automatically. Re-reading or editing never costs credits.

### Not in version 1

Private repositories (needs the GitHub App, version 1.1), two-factor authentication with an authenticator app and recovery codes (1.1), payments and plans, resume and portfolio generation, team workspaces, automatic updates when the repository changes, chat with a repository, and editing diagrams by hand.

## 4. Screens

Every screen has a loading state, an error state and a phone layout. `[ ]` marks something that exists today.

| # | Screen | Route | Purpose |
|---|---|---|---|
| 1 | Landing | `/` | What it is, how it works, a real sample output, sign-in button |
| 2 | Sign in `[ ]` | `/login` | Email code, Google, GitHub; plain error messages |
| 3 | Dashboard | `/dashboard` | Your projects as cards with status; "New project"; credit balance |
| 4 | New project | `/projects/new` | Paste a GitHub URL, see the repository checked, confirm |
| 5 | Project overview | `/projects/[id]` | Live progress while analysing; afterwards the summary, stack, entry points, file tree |
| 6 | README | `/projects/[id]/readme` | Section list, editor, preview, health score, history |
| 7 | Context pack | `/projects/[id]/context-pack` | Preset picker, preview, download |
| 8 | Diagram | `/projects/[id]/diagram` | Viewer, regenerate, download, embed snippet |
| 9 | Project settings | `/projects/[id]/settings` | Re-analyse, delete the project |
| 10 | Account `[ ]` | `/account` | Name, devices, connected accounts, danger zone (delete account) |
| 11 | Usage | `/usage` | Balance, history of every charge, refunds |
| 12 | Status `[ ]` | `/status` | Whether the services are healthy (moves from `/`) |
| 13 | Privacy and terms | `/privacy`, `/terms` | Plain-language legal pages |
| 14 | Not found, error, out of credits | | Friendly dead ends with a way forward |

### Dashboard

```
┌───────────────────────────────────────────────────────────────┐
│ Readmely   Projects   Usage                  ⚡ 55 credits  (A) │
├───────────────────────────────────────────────────────────────┤
│  Your projects                               [ + New project ] │
│                                                               │
│  ┌──────────────────┐  ┌──────────────────┐  ┌─────────────┐  │
│  │ owner/api-server │  │ owner/dotfiles   │  │ owner/blog  │  │
│  │ Go · Postgres    │  │ Shell            │  │ Analysing…  │  │
│  │ ● Ready          │  │ ● Ready          │  │ ███░░ 60%   │  │
│  │ README  Pack  ◇  │  │ README           │  │             │  │
│  └──────────────────┘  └──────────────────┘  └─────────────┘  │
└───────────────────────────────────────────────────────────────┘
```

### Project page

```
┌───────────────────────────────────────────────────────────────┐
│ ← Projects   owner/api-server        [Re-analyse]  [Settings] │
│ Overview │ README │ Context pack │ Diagram                    │
├───────────────────────────────────────────────────────────────┤
│  A REST API for … (plain-English summary)                     │
│                                                               │
│  Languages  ████████ Go 71%  ███ SQL 18%  ▌Other 11%          │
│  Stack      [Chi] [Postgres] [Redis] [Docker]                 │
│  Run        make up        Test   make test                   │
│  Entry      cmd/api/main.go                                   │
│                                                               │
│  Files ▸ cmd/  ▸ internal/  ▸ migrations/                     │
│  ⚠ 2 possible secrets were found and left out (see locations) │
└───────────────────────────────────────────────────────────────┘
```

### README editor

```
┌───────────────────────────────────────────────────────────────┐
│ README   Template: Standard ▾   Health 82/100   [Download]    │
├───────────────┬─────────────────────────┬─────────────────────┤
│ Sections      │ # api-server            │ api-server          │
│ ✓ Title       │ A REST API for …        │ A REST API for …    │
│ ✓ Overview    │                         │                     │
│ ✓ Install     │ ## Installation         │ Installation        │
│ ✓ Usage  ↻    │ ```sh                   │ $ make up           │
│ ✗ License  +  │ make up                 │                     │
│               │ ```                     │                     │
│ History ▾     │                         │ Missing: License    │
└───────────────┴─────────────────────────┴─────────────────────┘
   ↻ regenerate this section (2 credits)   + add a missing section
```

## 5. User flows

### 5.1 First visit to first README

1. Open the landing page, read the sample, click **Get started**.
2. `/login`: enter an email and the code from the inbox, or click Google or GitHub. A new account is created and 100 credits are granted.
3. The dashboard is empty and offers **New project**.
4. Paste `https://github.com/owner/repo`. The page checks the repository (exists, public, not too large) and shows its name, description and size. Click **Create and analyse**.
5. The project page shows live progress: *Fetching → Scanning for secrets → Reading code → Summarising*. This takes about a minute for a typical repository, and costs 10 credits.
6. The overview fills in. Click **README**, then **Generate**. Sections appear as they finish.
7. Edit anything in the editor, regenerate a weak section with an instruction, then **Download** or copy.

### 5.2 Context pack

Project page, **Context pack** tab, pick a preset (for example Claude Code), read the preview, check the token estimate, **Download ZIP**, and unzip into the repository root.

### 5.3 Diagram

Project page, **Diagram** tab, **Generate**, then pan and zoom; if it focuses on the wrong thing, type "show the worker and the queue" and **Regenerate**. Download SVG or PNG, or click **Copy Markdown** and paste it into the README (the README tab also has **Insert diagram**).

### 5.4 Running out of credits

An action that would cost more than the balance is blocked before it starts with "This needs 15 credits; you have 5", a link to the usage page, and no charge. Version 1 has no purchase button; the message says so and invites the user to get in touch.

### 5.5 When something fails

- A repository that is private, missing, or too large is explained at the paste step, before any credit is spent.
- A job that fails shows what step failed in plain words, offers **Try again**, and has already refunded its credits.
- If the model provider is down, the job is retried with the second provider; if both fail the user sees a calm "try again in a few minutes".

### 5.6 Leaving

`/account` → **Delete account** → enter the code sent by email → everything is deleted (projects, outputs, files, sessions) and a confirmation page is shown. Usage entries stay without any personal data.

## 6. Quality bar for the first release

- **Safe by construction:** repositories are cloned and read only in the locked-down worker, secrets are removed before any text goes to a model, repository content cannot give the model instructions, and one user can never see another's projects.
- **Honest outputs:** the README only claims commands and files that exist in the repository; the diagram only shows components found in it.
- **Fast enough:** a typical repository (under 5,000 files) analysed in about a minute, each output in under a minute more.
- **Accessible and responsive:** keyboard-friendly, labelled controls, readable on a phone.

## 7. What the repository will show a visitor

A README with screenshots and an architecture diagram generated by Readmely itself, a plans folder that records how decisions were made, typed commit messages, one pull request per phase with a summary of what changed and how it was checked, and passing CI on `main`.
