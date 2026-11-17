import "server-only";
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

/** Reads the body but gives up as soon as it exceeds `limit` bytes. */
async function readLimited(response: Response, limit: number): Promise<string> {
  if (!response.body) return "";
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > limit) {
      await reader.cancel();
      throw new Error("body too large");
    }
    chunks.push(value);
  }
  return new TextDecoder().decode(Buffer.concat(chunks));
}
