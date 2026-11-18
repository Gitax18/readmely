import "server-only";
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
export const MAX_RESPONSE_BYTES = 256 * 1024;

const UUID = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
const ALLOWED_PATHS = [
  /^v1\/auth\/email\/(start|verify)$/,
  /^v1\/auth\/logout$/,
  /^v1\/me$/,
  /^v1\/me\/sessions$/,
  new RegExp(`^v1/me/sessions/${UUID}$`),
];

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
    const declared = Number(request.headers.get("content-length") ?? "0");
    if (declared > MAX_REQUEST_BYTES) return problem(413, "payload_too_large");
    body = await request.arrayBuffer();
    if (body.byteLength > MAX_REQUEST_BYTES) return problem(413, "payload_too_large");
  }

  const doFetch = options.fetchImpl ?? fetch;
  let upstream: Response;
  try {
    upstream = await doFetch(`${options.apiPublicUrl}/${path}`, {
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
