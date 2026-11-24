import "server-only";
import {
  gitHubAccessSchema,
  identityListSchema,
  meSchema,
  providersSchema,
  sessionListSchema,
  type GitHubAccess,
  type Identity,
  type Me,
  type Session,
} from "./account-schema";
import { creditBalanceSchema, creditHistorySchema, type CreditEntry } from "./credit-schema";
import { projectListSchema, projectSchema, type Project } from "./project-schema";
import type { Provider } from "./providers";
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

/**
 * Which providers are configured. The login page shows a button for each. When the API cannot be
 * asked, no buttons appear and email codes still work, so this never throws and never blocks.
 */
export async function fetchProviders(baseUrl: string, options: Options = {}): Promise<Provider[]> {
  const doFetch = options.fetchImpl ?? fetch;
  try {
    const response = await doFetch(`${baseUrl}/v1/auth/providers`, {
      cache: "no-store",
      redirect: "error",
      headers: { Accept: "application/json" },
      signal: AbortSignal.timeout(options.timeoutMs ?? FETCH_TIMEOUT_MS),
    });
    if (!response.ok) return [];
    const parsed = providersSchema.safeParse(
      JSON.parse(await readLimited(response, MAX_BODY_BYTES)),
    );
    return parsed.success ? parsed.data.providers : [];
  } catch {
    return [];
  }
}

/** The signed-in user's connected provider accounts, or null when they cannot be loaded. */
export async function fetchIdentities(
  baseUrl: string,
  sessionToken: string,
  options: Options = {},
): Promise<Identity[] | null> {
  const res = await getJson(baseUrl, "/v1/me/identities", sessionToken, options);
  if (res === null || res.status === 401) return null;
  const parsed = identityListSchema.safeParse(res.json);
  return parsed.success ? parsed.data.identities : null;
}

/** Whether private GitHub repositories can be read for this user, or null when that cannot be told. */
export async function fetchGitHubAccess(
  baseUrl: string,
  sessionToken: string,
  options: Options = {},
): Promise<GitHubAccess | null> {
  const res = await getJson(baseUrl, "/v1/me/github/access", sessionToken, options);
  if (res === null || res.status === 401) return null;
  const parsed = gitHubAccessSchema.safeParse(res.json);
  return parsed.success ? parsed.data : null;
}

/** The signed-in user's projects, or null when they cannot be loaded. */
export async function fetchProjects(
  baseUrl: string,
  sessionToken: string,
  options: Options = {},
): Promise<Project[] | null> {
  const res = await getJson(baseUrl, "/v1/projects", sessionToken, options);
  if (res === null || res.status === 401) return null;
  const parsed = projectListSchema.safeParse(res.json);
  return parsed.success ? parsed.data.projects : null;
}

/** One of the user's projects; "missing" when it does not exist or is not theirs. */
export async function fetchProject(
  baseUrl: string,
  sessionToken: string,
  id: string,
  options: Options = {},
): Promise<Project | "missing" | null> {
  const doFetch = options.fetchImpl ?? fetch;
  try {
    const response = await doFetch(`${baseUrl}/v1/projects/${encodeURIComponent(id)}`, {
      cache: "no-store",
      redirect: "error",
      headers: { Accept: "application/json", Cookie: `${SESSION_COOKIE}=${sessionToken}` },
      signal: AbortSignal.timeout(options.timeoutMs ?? FETCH_TIMEOUT_MS),
    });
    if (response.status === 404) return "missing";
    if (!response.ok) return null;
    const parsed = projectSchema.safeParse(JSON.parse(await readLimited(response, MAX_BODY_BYTES)));
    return parsed.success ? parsed.data : null;
  } catch {
    return null;
  }
}

/** The signed-in user's credit balance, or null when it cannot be loaded. */
export async function fetchCredits(
  baseUrl: string,
  sessionToken: string,
  options: Options = {},
): Promise<number | null> {
  const res = await getJson(baseUrl, "/v1/me/credits", sessionToken, options);
  if (res === null || res.status === 401) return null;
  const parsed = creditBalanceSchema.safeParse(res.json);
  return parsed.success ? parsed.data.balance : null;
}

/** The most recent credit movements, newest first, or null when they cannot be loaded. */
export async function fetchCreditHistory(
  baseUrl: string,
  sessionToken: string,
  options: Options = {},
): Promise<CreditEntry[] | null> {
  const res = await getJson(baseUrl, "/v1/me/credits/history", sessionToken, options);
  if (res === null || res.status === 401) return null;
  const parsed = creditHistorySchema.safeParse(res.json);
  return parsed.success ? parsed.data.entries : null;
}
