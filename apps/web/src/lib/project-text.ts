import type { Failure } from "./browser-api";
import type { Project } from "./project-schema";

/** What the person sees when a projects request fails. Never provider or server text. */
export function projectFailureText(f: Failure): string {
  switch (f.kind) {
    case "invalid_request":
      return "That does not look like a GitHub repository address. Use https://github.com/owner/repo.";
    case "repository_not_found":
      return "We could not find a public repository there. Check the address; private repositories are not supported yet.";
    case "repository_too_large":
      return "That repository is too large for Readmely (over 200 MB).";
    case "github_unavailable":
      return "GitHub is not answering right now. Try again in a minute.";
    case "already_added":
      return "You already added that repository.";
    case "project_limit":
      return "You have 10 projects, the most for now. Delete one to add another.";
    case "analysis_running":
      return "An analysis is already running for this project.";
    case "insufficient_credits":
      return "You do not have enough credits for that. See Usage for your balance.";
    case "not_analysed":
      return "Analyse the project first.";
    case "readme_running":
      return "A README is already being written for this project.";
    case "diagram_running":
      return "A diagram is already being drawn for this project.";
    case "stale_version":
      return "The README changed since you opened it. Reload the page, then make your edit again.";
    case "rate_limited":
      return "Too many requests. Try again shortly.";
    case "forbidden":
      return "This page is out of date. Reload it and try again.";
    case "not_found":
      return "That project no longer exists.";
    default:
      return "Something went wrong on our side. Please try again in a moment.";
  }
}

const STEP_LABELS: Record<string, string> = {
  queued: "Waiting for a worker",
  fetching: "Fetching the repository",
  scanning: "Scanning for secrets",
  parsing: "Reading the code",
  mapping: "Mapping how the code fits together",
  saving: "Saving what we found",
  summarising: "Summarising",
};

export function stepLabel(step: string | null): string {
  return (step !== null && STEP_LABELS[step]) || "Waiting for a worker";
}

/** Why an analysis failed, in plain words. */
export function jobFailureText(code: string | null): string {
  switch (code) {
    case "lease_expired":
      return "The analysis was interrupted before it finished.";
    case "repository_not_found":
      return "We could not find the repository any more. It may have been renamed, made private or deleted.";
    case "repository_too_large":
      return "This repository is too large to analyse.";
    case "repository_empty":
      return "The repository has no files to analyse.";
    case "github_unavailable":
      return "GitHub did not answer in time.";
    case "archive_unreadable":
      return "We could not read the repository's files.";
    default:
      return "The analysis hit a problem on our side.";
  }
}

export type StatusTone = "ok" | "warn" | "bad" | "neutral";

export function statusBadge(p: Project): { label: string; tone: StatusTone } {
  switch (p.status) {
    case "ready":
      return { label: "Ready", tone: "ok" };
    case "importing":
      return { label: "Analysing", tone: "warn" };
    case "failed":
      return { label: "Failed", tone: "bad" };
    default:
      return { label: "Not analysed", tone: "neutral" };
  }
}

const OWNER = /^[A-Za-z0-9][A-Za-z0-9-]{0,38}$/;
const REPO = /^[A-Za-z0-9._-]{1,100}$/;

/** The repository's page on GitHub, or null for a project without a well-formed owner and name. */
export function repoUrl(p: Pick<Project, "owner" | "repo">): string | null {
  return OWNER.test(p.owner) && REPO.test(p.repo) && p.repo !== "." && p.repo !== ".."
    ? `https://github.com/${p.owner}/${p.repo}`
    : null;
}

export function formatSize(kb: number): string {
  if (kb < 1024) return `${kb} KB`;
  return `${(kb / 1024).toFixed(kb < 10 * 1024 ? 1 : 0)} MB`;
}

/** Plain names for the kinds of secret the scanner removes. */
const SECRET_RULES: Record<string, string> = {
  private_key: "Private key",
  aws_access_key: "AWS access key",
  github_token: "GitHub token",
  slack_token: "Slack token",
  google_api_key: "Google API key",
  stripe_key: "Stripe key",
  api_key: "API key",
  jwt: "Signed token (JWT)",
  credential_url: "Password in an address",
  assigned_secret: "Secret assigned in code",
};

export function secretRuleLabel(rule: string): string {
  return SECRET_RULES[rule] ?? "Secret";
}

const SKIP_REASONS: Record<string, string> = {
  binary: "images and other binary files",
  too_large: "files over 256 KB",
  sensitive: "files that usually hold credentials",
  generated: "lock files and generated code",
  vendored: "installed dependencies and build output",
  budget: "files beyond the size budget",
};

export function skipReasonLabel(reason: string): string {
  return SKIP_REASONS[reason] ?? "other files";
}

export function formatCount(n: number): string {
  return new Intl.NumberFormat("en-US").format(n);
}

const SUMMARY_STEPS: Record<string, string> = {
  reading: "Reading the project",
  writing: "Writing the summary",
  saving: "Saving the summary",
};

export function summaryStepLabel(step: string | null): string {
  return (step !== null && SUMMARY_STEPS[step]) || "Waiting for the writer";
}

/** Why there is no summary, in plain words. Credits are never lost: a failed summary is refunded. */
export function summaryFailureText(code: string | null): string {
  switch (code) {
    case "insufficient_credits":
      return "There were not enough credits to write the summary. Writing it holds 10 credits; see Usage.";
    case "spend_cap_reached":
      return "Readmely has reached its limit for AI work today. Analyse again tomorrow. Nothing was charged.";
    case "llm_not_configured":
      return "AI summaries are not switched on for this server. Nothing was charged.";
    case "stale_analysis":
      return "The project was analysed again while the summary was being written. Nothing was charged.";
    default:
      return "The summary could not be written this time. Nothing was charged. Analyse again to retry.";
  }
}

/** Why a README job failed, in plain words. Credits are never lost: a failed job is refunded. */
export function readmeFailureText(code: string | null): string {
  switch (code) {
    case "insufficient_credits":
      return "There were not enough credits.";
    case "spend_cap_reached":
      return "Readmely has reached its limit for AI work today. Try again tomorrow. Nothing was charged.";
    case "llm_not_configured":
      return "AI writing is not switched on for this server. Nothing was charged.";
    case "section_missing":
      return "That section is no longer in the README. Nothing was charged.";
    default:
      return "The README could not be written this time. Nothing was charged. Try again.";
  }
}

const README_STEPS: Record<string, string> = {
  reading: "Reading the project",
  writing: "Writing",
  saving: "Saving",
};

export function readmeStepLabel(step: string | null): string {
  return (step !== null && README_STEPS[step]) || "Waiting for the writer";
}

/** Why a diagram job failed, in plain words. Credits are never lost: a failed job is refunded. */
export function diagramFailureText(code: string | null): string {
  switch (code) {
    case "insufficient_credits":
      return "There were not enough credits.";
    case "spend_cap_reached":
      return "Readmely has reached its limit for AI work today. Try again tomorrow. Nothing was charged.";
    case "llm_not_configured":
      return "AI drawing is not switched on for this server. Nothing was charged.";
    case "llm_invalid_output":
    case "diagram_unusable":
      return "The diagram did not come out clean enough to show. Nothing was charged. Try again.";
    default:
      return "The diagram could not be drawn this time. Nothing was charged. Try again.";
  }
}

const DIAGRAM_STEPS: Record<string, string> = {
  reading: "Reading the project",
  writing: "Working out the parts",
  drawing: "Drawing",
  saving: "Saving",
};

export function diagramStepLabel(step: string | null): string {
  return (step !== null && DIAGRAM_STEPS[step]) || "Waiting for the artist";
}
