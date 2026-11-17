/**
 * Builds the Content-Security-Policy for one request. A fresh nonce is
 * required for every request: scripts run only if they carry it, and
 * `strict-dynamic` lets those trusted scripts load their own chunks.
 *
 * Development adds 'unsafe-eval' (React uses it for debugging); production
 * never allows eval or inline script. `upgrade-insecure-requests` is left out
 * until the site is served over TLS (added with HSTS at deploy time).
 */
export function buildCsp(nonce: string, isDev: boolean): string {
  const directives = [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${isDev ? " 'unsafe-eval'" : ""}`,
    `style-src 'self' 'nonce-${nonce}'`,
    "img-src 'self' data:",
    "font-src 'self'",
    // Dev tooling (hot reload) needs a websocket back to the dev server.
    `connect-src 'self'${isDev ? " ws: wss:" : ""}`,
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ];
  return directives.join("; ");
}

/** 128 bits of randomness, base64-encoded: unpredictable and unique per call. */
export function newNonce(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return btoa(String.fromCharCode(...bytes));
}
