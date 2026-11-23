/**
 * The sign-in providers, and where the browser may be sent to approve one. Shared by the bridge
 * (which refuses any other redirect) and the browser client (which refuses to navigate anywhere
 * else), so a bug or a hostile response cannot turn "Connect" into an open redirect.
 */

export const PROVIDERS = ["google", "github"] as const;
export type Provider = (typeof PROVIDERS)[number];

export const PROVIDER_LABELS: Record<Provider, string> = { google: "Google", github: "GitHub" };

const AUTHORIZATION_ENDPOINTS: readonly { origin: string; pathname: string }[] = [
  { origin: "https://accounts.google.com", pathname: "/o/oauth2/v2/auth" },
  { origin: "https://github.com", pathname: "/login/oauth/authorize" },
];

const MAX_URL_LENGTH = 2048;

/** True only for an https authorization URL at one of the providers' real endpoints. */
export function isProviderAuthorizationUrl(value: string): boolean {
  if (value.length > MAX_URL_LENGTH) return false;
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return false;
  }
  return (
    url.username === "" &&
    url.password === "" &&
    url.hash === "" &&
    AUTHORIZATION_ENDPOINTS.some((e) => url.origin === e.origin && url.pathname === e.pathname)
  );
}

// A site-relative path such as /account or /login?error=access_denied: one leading slash, then
// only URL-safe characters. No scheme, no host, no backslash, no control characters.
const SAFE_PATH = /^\/(?![/\\])[A-Za-z0-9\-._~!$&'()*+,;=:@%/?]*$/;

/** True for a same-site path the API may redirect the browser to. */
export function isSiteRelativePath(value: string): boolean {
  return value.length <= MAX_URL_LENGTH && SAFE_PATH.test(value) && !value.includes("..");
}
