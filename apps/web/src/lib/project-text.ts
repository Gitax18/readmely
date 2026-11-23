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
