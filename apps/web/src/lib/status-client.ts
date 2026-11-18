import "server-only";
import { readLimited } from "./read-limited";
import { reportSchema, type Report } from "./status-schema";

export const FETCH_TIMEOUT_MS = 3000;
export const MAX_BODY_BYTES = 64 * 1024;

export type StatusResult =
  { kind: "report"; report: Report } | { kind: "api_unreachable" } | { kind: "invalid" };

/**
 * Fetches the combined status from the Go API. Never throws: every failure
 * becomes a result the page can show. Failure details are not exposed.
 */
export async function fetchStatus(
  baseUrl: string,
  options: { fetchImpl?: typeof fetch; timeoutMs?: number } = {},
): Promise<StatusResult> {
  const doFetch = options.fetchImpl ?? fetch;
  const timeoutMs = options.timeoutMs ?? FETCH_TIMEOUT_MS;

  let response: Response;
  try {
    response = await doFetch(`${baseUrl}/api/v1/status`, {
      cache: "no-store",
      redirect: "error",
      headers: { Accept: "application/json" },
      signal: AbortSignal.timeout(timeoutMs),
    });
  } catch {
    return { kind: "api_unreachable" };
  }

  if (!response.ok) return { kind: "api_unreachable" };

  let text: string;
  try {
    text = await readLimited(response, MAX_BODY_BYTES);
  } catch {
    return { kind: "invalid" };
  }

  let json: unknown;
  try {
    json = JSON.parse(text);
  } catch {
    return { kind: "invalid" };
  }
  const parsed = reportSchema.safeParse(json);
  return parsed.success ? { kind: "report", report: parsed.data } : { kind: "invalid" };
}
