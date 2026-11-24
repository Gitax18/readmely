/** github.com/owner/repo only: the same rule the API applies before it looks a repository up. */
export const GITHUB_REPO = /^https:\/\/github\.com\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\/?$/;

export const INVALID_REPO =
  "That does not look like a GitHub repository address. Use https://github.com/owner/repo.";

/**
 * A repository address carried in the query string (from the landing page, through sign-in, to the
 * new-project form). Anything that is not a plain GitHub repository address is dropped, so the
 * value is safe to put back into a link or a field.
 */
export function repoParam(value: string | string[] | undefined): string | null {
  const v = typeof value === "string" ? value.trim() : "";
  return v.length <= 300 && GITHUB_REPO.test(v) ? v : null;
}
