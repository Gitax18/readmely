import { z } from "zod";
import {
  authorizationUrlSchema,
  identityListSchema,
  meSchema,
  sessionListSchema,
  userSchema,
  type Identity,
  type Me,
  type Session,
  type User,
} from "./account-schema";
import { isProviderAuthorizationUrl, type Provider } from "./providers";

/**
 * Calls the account API from the browser, always through the same-origin /api bridge.
 * Every function returns a result instead of throwing, so screens can show a clear message.
 */

export type Failure =
  | { ok: false; kind: "invalid_request" }
  | { ok: false; kind: "invalid_code" }
  | { ok: false; kind: "rate_limited"; retryAfterSeconds: number }
  | { ok: false; kind: "unauthenticated" }
  | { ok: false; kind: "forbidden" }
  | { ok: false; kind: "not_found" }
  | { ok: false; kind: "conflict" }
  | { ok: false; kind: "repository_not_found" }
  | { ok: false; kind: "repository_too_large" }
  | { ok: false; kind: "github_unavailable" }
  | { ok: false; kind: "already_added" }
  | { ok: false; kind: "project_limit" }
  | { ok: false; kind: "analysis_running" }
  | { ok: false; kind: "insufficient_credits" }
  | { ok: false; kind: "not_analysed" }
  | { ok: false; kind: "readme_running" }
  | { ok: false; kind: "diagram_running" }
  | { ok: false; kind: "stale_version" }
  | { ok: false; kind: "unavailable" };

export type Result<T> = ({ ok: true } & T) | Failure;

const BASE = "/api/v1";
const DEFAULT_RETRY_SECONDS = 60;

export async function request(
  fetchImpl: typeof fetch,
  method: string,
  path: string,
  options: { body?: unknown; csrf?: string } = {},
): Promise<Response | null> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (options.csrf !== undefined) headers["X-CSRF-Token"] = options.csrf;
  try {
    return await fetchImpl(`${BASE}${path}`, {
      method,
      headers,
      credentials: "same-origin",
      cache: "no-store",
      ...(options.body !== undefined ? { body: JSON.stringify(options.body) } : {}),
    });
  } catch {
    return null;
  }
}

export async function failure(res: Response | null): Promise<Failure> {
  if (res === null) return { ok: false, kind: "unavailable" };
  let code = "";
  try {
    const body: unknown = await res.json();
    code = z.object({ error: z.string() }).parse(body).error;
  } catch {
    // An unreadable body is treated by status alone below.
  }
  if (res.status === 429) {
    const seconds = Number.parseInt(res.headers.get("retry-after") ?? "", 10);
    return {
      ok: false,
      kind: "rate_limited",
      retryAfterSeconds: Number.isFinite(seconds) && seconds > 0 ? seconds : DEFAULT_RETRY_SECONDS,
    };
  }
  if (res.status === 401) return { ok: false, kind: "unauthenticated" };
  if (res.status === 403) return { ok: false, kind: "forbidden" };
  if (res.status === 404 && code === "repository_not_found") {
    return { ok: false, kind: "repository_not_found" };
  }
  if (res.status === 404) return { ok: false, kind: "not_found" };
  if (res.status === 409 && code === "already_added") return { ok: false, kind: "already_added" };
  if (res.status === 409 && code === "project_limit") return { ok: false, kind: "project_limit" };
  if (res.status === 409 && code === "analysis_running") {
    return { ok: false, kind: "analysis_running" };
  }
  if (res.status === 402) return { ok: false, kind: "insufficient_credits" };
  if (res.status === 409 && code === "not_analysed") return { ok: false, kind: "not_analysed" };
  if (res.status === 409 && code === "readme_running") return { ok: false, kind: "readme_running" };
  if (res.status === 409 && code === "diagram_running")
    return { ok: false, kind: "diagram_running" };
  if (res.status === 409 && code === "stale_version") return { ok: false, kind: "stale_version" };
  if (res.status === 409) return { ok: false, kind: "conflict" };
  if (res.status === 503 && code === "github_unavailable") {
    return { ok: false, kind: "github_unavailable" };
  }
  if (res.status === 400 && code === "repository_too_large") {
    return { ok: false, kind: "repository_too_large" };
  }
  if (res.status === 400 && code === "invalid_code") return { ok: false, kind: "invalid_code" };
  if (res.status === 400) return { ok: false, kind: "invalid_request" };
  return { ok: false, kind: "unavailable" };
}

export async function parsed<T>(res: Response, schema: z.ZodType<T>): Promise<T | null> {
  try {
    const result = schema.safeParse(await res.json());
    return result.success ? result.data : null;
  } catch {
    return null;
  }
}

export interface AccountApi {
  startLogin(email: string): Promise<Result<object>>;
  verifyLogin(email: string, code: string): Promise<Result<{ me: Me }>>;
  logout(csrf: string): Promise<Result<object>>;
  rename(csrf: string, displayName: string): Promise<Result<{ user: User }>>;
  listSessions(): Promise<Result<{ sessions: Session[] }>>;
  revokeSession(csrf: string, id: string): Promise<Result<object>>;
  revokeOtherSessions(csrf: string): Promise<Result<object>>;
  /** Emails a code to the account's own address; deleting needs it. */
  startDeletion(csrf: string): Promise<Result<object>>;
  confirmDeletion(csrf: string, code: string): Promise<Result<object>>;
  listIdentities(): Promise<Result<{ identities: Identity[] }>>;
  /** Where to send the browser to connect a provider; only ever a provider's own authorization URL. */
  startConnect(csrf: string, provider: Provider): Promise<Result<{ url: string }>>;
  disconnect(csrf: string, provider: Provider): Promise<Result<object>>;
}

export function createAccountApi(
  fetchImpl: typeof fetch = (...args) => fetch(...args),
): AccountApi {
  return {
    async startLogin(email) {
      const res = await request(fetchImpl, "POST", "/auth/email/start", { body: { email } });
      return res?.status === 202 ? { ok: true } : failure(res);
    },
    async verifyLogin(email, code) {
      const res = await request(fetchImpl, "POST", "/auth/email/verify", { body: { email, code } });
      if (res?.status !== 200) return failure(res);
      const me = await parsed(res, meSchema);
      return me ? { ok: true, me } : { ok: false, kind: "unavailable" };
    },
    async logout(csrf) {
      const res = await request(fetchImpl, "POST", "/auth/logout", { csrf });
      return res?.status === 204 ? { ok: true } : failure(res);
    },
    async rename(csrf, displayName) {
      const res = await request(fetchImpl, "PATCH", "/me", {
        body: { display_name: displayName },
        csrf,
      });
      if (res?.status !== 200) return failure(res);
      const user = await parsed(res, userSchema);
      return user ? { ok: true, user } : { ok: false, kind: "unavailable" };
    },
    async listSessions() {
      const res = await request(fetchImpl, "GET", "/me/sessions");
      if (res?.status !== 200) return failure(res);
      const list = await parsed(res, sessionListSchema);
      return list ? { ok: true, sessions: list.sessions } : { ok: false, kind: "unavailable" };
    },
    async revokeSession(csrf, id) {
      const res = await request(fetchImpl, "DELETE", `/me/sessions/${encodeURIComponent(id)}`, {
        csrf,
      });
      return res?.status === 204 ? { ok: true } : failure(res);
    },
    async revokeOtherSessions(csrf) {
      const res = await request(fetchImpl, "DELETE", "/me/sessions", { csrf });
      return res?.status === 204 ? { ok: true } : failure(res);
    },
    async startDeletion(csrf) {
      const res = await request(fetchImpl, "POST", "/me/deletion/start", { csrf });
      return res?.status === 202 ? { ok: true } : failure(res);
    },
    async confirmDeletion(csrf, code) {
      const res = await request(fetchImpl, "POST", "/me/deletion/confirm", {
        body: { code },
        csrf,
      });
      return res?.status === 204 ? { ok: true } : failure(res);
    },
    async listIdentities() {
      const res = await request(fetchImpl, "GET", "/me/identities");
      if (res?.status !== 200) return failure(res);
      const list = await parsed(res, identityListSchema);
      return list ? { ok: true, identities: list.identities } : { ok: false, kind: "unavailable" };
    },
    async startConnect(csrf, provider) {
      const res = await request(fetchImpl, "POST", `/me/identities/${provider}/start`, { csrf });
      if (res?.status !== 200) return failure(res);
      const body = await parsed(res, authorizationUrlSchema);
      // Never navigate anywhere that is not the provider's own authorization endpoint.
      return body && isProviderAuthorizationUrl(body.authorization_url)
        ? { ok: true, url: body.authorization_url }
        : { ok: false, kind: "unavailable" };
    },
    async disconnect(csrf, provider) {
      const res = await request(fetchImpl, "DELETE", `/me/identities/${provider}`, { csrf });
      return res?.status === 204 ? { ok: true } : failure(res);
    },
  };
}
