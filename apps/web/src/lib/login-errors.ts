/**
 * What /login?error=CODE means. The codes are fixed by the API (contracts/core-public.yaml); the
 * page never shows anything the provider sent, so an unknown code shows nothing at all.
 */
const MESSAGES: Readonly<Record<string, string>> = {
  access_denied: "Sign-in was cancelled. You can try again, or use an emailed code.",
  invalid_state: "That sign-in expired or was already used. Please start again.",
  no_verified_email:
    "Your provider did not give us a verified email address. Verify it with the provider, or use an emailed code.",
  provider_conflict:
    "This email already has a different account of that provider connected. Sign in with an emailed code, then manage your connected accounts.",
  identity_in_use: "That provider account is already connected to another Readmely account.",
  unsupported_email:
    "We could not use the email address your provider gave us. Use an emailed code instead.",
  provider_error: "The provider did not answer as expected. Please try again in a moment.",
  account_unavailable: "This account cannot sign in right now.",
};

/** The message for a code from the URL, or null when the code is not one of ours. */
export function loginErrorMessage(code: string | string[] | undefined): string | null {
  if (typeof code !== "string") return null;
  return Object.hasOwn(MESSAGES, code) ? (MESSAGES[code] ?? null) : null;
}
