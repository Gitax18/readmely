import "server-only";
import { meSchema, sessionListSchema, type Me, type Session } from "./account-schema";
import { readLimited } from "./read-limited";

export const FETCH_TIMEOUT_MS = 3000;
export const MAX_BODY_BYTES = 64 * 1024;

export const SESSION_COOKIE = "__Host-rm_session";

export type SessionResult =
  { kind: "signed_in"; me: Me } | { kind: "signed_out" } | { kind: "error" };

interface Options {
  fetchImpl?: typeof fetch;
  timeoutMs?: number;
}

async function getJson(
  baseUrl: string,
  path: string,
  sessionToken: string,
  options: Options,
): Promise<{ status: number; json: unknown } | null> {
  const doFetch = options.fetchImpl ?? fetch;
  try {
    const response = await doFetch(`${baseUrl}${path}`, {
      cache: "no-store",
      redirect: "error",
      headers: { Accept: "application/json", Cookie: `${SESSION_COOKIE}=${sessionToken}` },
      signal: AbortSignal.timeout(options.timeoutMs ?? FETCH_TIMEOUT_MS),
    });
    if (response.status === 401) return { status: 401, json: null };
    if (!response.ok) return null;
    return {
      status: response.status,
      json: JSON.parse(await readLimited(response, MAX_BODY_BYTES)),
    };
  } catch {
    return null;
  }
}

/**
 * Asks the API who the cookie belongs to. Runs on the server, so a signed-out visitor is
 * redirected before any account page is sent; the browser is never trusted to say it is signed in.
 * Never throws: an unreachable or confused API is "error", which is not the same as signed out.
 */
export async function fetchSession(
  baseUrl: string,
  sessionToken: string | undefined,
  options: Options = {},
): Promise<SessionResult> {
  if (!sessionToken) return { kind: "signed_out" };
  const res = await getJson(baseUrl, "/v1/me", sessionToken, options);
  if (res === null) return { kind: "error" };
  if (res.status === 401) return { kind: "signed_out" };
  const parsed = meSchema.safeParse(res.json);
  return parsed.success ? { kind: "signed_in", me: parsed.data } : { kind: "error" };
}

/** The signed-in user's sessions, or null when they cannot be loaded. */
export async function fetchSessions(
  baseUrl: string,
  sessionToken: string,
  options: Options = {},
): Promise<Session[] | null> {
  const res = await getJson(baseUrl, "/v1/me/sessions", sessionToken, options);
  if (res === null || res.status === 401) return null;
  const parsed = sessionListSchema.safeParse(res.json);
  return parsed.success ? parsed.data.sessions : null;
}
