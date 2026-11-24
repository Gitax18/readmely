import "server-only";
import { isProviderAuthorizationUrl, isSiteRelativePath } from "./providers";
import { readLimited } from "./read-limited";

/**
 * The same-origin /api/* bridge to the Go API's PUBLIC listener.
 *
 * Why a handler and not a Next.js rewrite: rewrites are fixed at build time, cannot filter what
 * they pass on, and cannot decide what to do with X-Forwarded-For. This forwards only the account
 * endpoints (audit F14: the internal listener's /readyz and status detail must never be reachable
 * from a browser), only the headers the API needs, and only the response headers the browser needs.
 */

export const REQUEST_TIMEOUT_MS = 10_000;
export const MAX_REQUEST_BYTES = 16 * 1024;
/** A README is up to 100 KiB, so saving one may send more than the usual limit; only that path may. */
export const README_REQUEST_BYTES = 128 * 1024;
export const MAX_RESPONSE_BYTES = 256 * 1024;

const UUID = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
const README_SAVE_PATH = new RegExp(`^v1/projects/${UUID}/readme$`);
const ALLOWED_PATHS = [
  /^v1\/auth\/email\/(start|verify)$/,
  /^v1\/auth\/logout$/,
  /^v1\/me$/,
  /^v1\/me\/sessions$/,
  new RegExp(`^v1/me/sessions/${UUID}$`),
  /^v1\/me\/deletion\/(start|confirm)$/,
  /^v1\/projects$/,
  /^v1\/projects\/preview$/,
  new RegExp(`^v1/projects/${UUID}$`),
  new RegExp(`^v1/projects/${UUID}/analyse$`),
  new RegExp(`^v1/projects/${UUID}/analysis$`),
  new RegExp(`^v1/projects/${UUID}/context-pack$`),
  new RegExp(`^v1/projects/${UUID}/diagram$`),
  new RegExp(`^v1/projects/${UUID}/readme$`),
  new RegExp(`^v1/projects/${UUID}/readme/sections$`),
  new RegExp(`^v1/projects/${UUID}/readme/versions$`),
  new RegExp(`^v1/projects/${UUID}/readme/versions/[1-9][0-9]{0,8}/restore$`),
  /^v1\/auth\/providers$/,
  /^v1\/auth\/(google|github)\/(start|callback)$/,
  /^v1\/me\/identities$/,
  /^v1\/me\/identities\/(google|github)$/,
  /^v1\/me\/identities\/(google|github)\/start$/,
  // Private GitHub repositories: the access state, granting it, and the list it unlocks.
  /^v1\/me\/github\/access$/,
  /^v1\/me\/github\/access\/start$/,
  /^v1\/me\/github\/repos$/,
];

// Only these two answer with a redirect, and only the callback takes a query.
const REDIRECTING_PATH = /^v1\/auth\/(google|github)\/(start|callback)$/;
const CALLBACK_PATH = /^v1\/auth\/(google|github)\/callback$/;
const CALLBACK_QUERY_LIMITS: Readonly<Record<string, number>> = {
  code: 2048,
  state: 256,
  error: 128,
};
const PRINTABLE_ASCII = /^[\x21-\x7e]+$/;

const FORWARD_REQUEST_HEADERS = [
  "accept",
  "content-type",
  "cookie",
  "origin",
  "user-agent",
  "x-csrf-token",
];
const FORWARD_RESPONSE_HEADERS = ["content-type", "retry-after", "x-request-id"];

export interface ProxyOptions {
  apiPublicUrl: string;
  behindProxy: boolean;
  fetchImpl?: typeof fetch;
  timeoutMs?: number;
}

function problem(status: number, error: string): Response {
  return new Response(
    JSON.stringify({ error, request_id: crypto.randomUUID().replaceAll("-", "") }),
    {
      status,
      headers: { "content-type": "application/json", "cache-control": "no-store" },
    },
  );
}

/**
 * The query the callback forwards: code, state and error, each bounded and printable ASCII, and
 * nothing else. Whatever else a request carries is dropped before the API sees it.
 */
export function callbackQuery(params: URLSearchParams): string {
  const out = new URLSearchParams();
  for (const [name, limit] of Object.entries(CALLBACK_QUERY_LIMITS)) {
    const value = params.get(name);
    if (value !== null && value.length <= limit && PRINTABLE_ASCII.test(value)) {
      out.set(name, value);
    }
  }
  const text = out.toString();
  return text === "" ? "" : `?${text}`;
}

/** A redirect target the browser may be given: a same-site path or a provider's authorization URL. */
export function safeLocation(value: string | null): string | null {
  if (value === null) return null;
  return isSiteRelativePath(value) || isProviderAuthorizationUrl(value) ? value : null;
}

/** Forwards one browser request to the API and returns its answer. Never throws. */
export async function proxyToApi(
  request: Request,
  pathSegments: string[],
  options: ProxyOptions,
): Promise<Response> {
  const path = pathSegments.join("/");
  if (!ALLOWED_PATHS.some((re) => re.test(path))) return problem(404, "not_found");

  const headers = new Headers();
  for (const name of FORWARD_REQUEST_HEADERS) {
    const value = request.headers.get(name);
    if (value !== null) headers.set(name, value);
  }
  if (options.behindProxy) {
    const forwarded = request.headers.get("x-forwarded-for");
    if (forwarded !== null) headers.set("x-forwarded-for", forwarded);
  }

  let body: ArrayBuffer | undefined;
  if (request.method !== "GET" && request.method !== "HEAD") {
    const limit =
      request.method === "PUT" && README_SAVE_PATH.test(path)
        ? README_REQUEST_BYTES
        : MAX_REQUEST_BYTES;
    const declared = Number(request.headers.get("content-length") ?? "0");
    if (declared > limit) return problem(413, "payload_too_large");
    body = await request.arrayBuffer();
    if (body.byteLength > limit) return problem(413, "payload_too_large");
  }

  const doFetch = options.fetchImpl ?? fetch;
  let upstream: Response;
  try {
    const query = CALLBACK_PATH.test(path) ? callbackQuery(new URL(request.url).searchParams) : "";
    upstream = await doFetch(`${options.apiPublicUrl}/${path}${query}`, {
      method: request.method,
      headers,
      ...(body !== undefined && body.byteLength > 0 ? { body } : {}),
      redirect: "manual",
      cache: "no-store",
      signal: AbortSignal.timeout(options.timeoutMs ?? REQUEST_TIMEOUT_MS),
    });
  } catch {
    return problem(502, "bad_gateway");
  }

  // A redirect is passed on only from the two provider routes, only as a 302, and only to a place
  // the API is supposed to send people. Anything else is a confused or compromised upstream.
  if (upstream.status >= 300 && upstream.status < 400) {
    const location = safeLocation(upstream.headers.get("location"));
    if (!REDIRECTING_PATH.test(path) || upstream.status !== 302 || location === null) {
      return problem(502, "bad_gateway");
    }
    const headers302 = new Headers({ "cache-control": "no-store", location });
    for (const cookie of upstream.headers.getSetCookie()) headers302.append("set-cookie", cookie);
    return new Response(null, { status: 302, headers: headers302 });
  }

  let text: string;
  try {
    text = upstream.status === 204 ? "" : await readLimited(upstream, MAX_RESPONSE_BYTES);
  } catch {
    return problem(502, "bad_gateway");
  }

  const out = new Headers({ "cache-control": "no-store" });
  for (const name of FORWARD_RESPONSE_HEADERS) {
    const value = upstream.headers.get(name);
    if (value !== null) out.set(name, value);
  }
  for (const cookie of upstream.headers.getSetCookie()) out.append("set-cookie", cookie);
  return new Response(upstream.status === 204 || text === "" ? null : text, {
    status: upstream.status,
    headers: out,
  });
}
