import "server-only";

/**
 * In production the API sits behind a reverse proxy that answers only requests carrying this
 * shared key, so nobody but this web server can reach it (see docs/deploy/aws.md). The key is
 * server-only and is never sent to a browser. Unset (local development) adds no header.
 */
export const ORIGIN_KEY_HEADER = "x-readmely-origin-key";

export function originKeyHeaders(
  source: Record<string, string | undefined> = process.env,
): Record<string, string> {
  const key = (source.API_ORIGIN_KEY ?? "").trim();
  return key === "" ? {} : { [ORIGIN_KEY_HEADER]: key };
}
