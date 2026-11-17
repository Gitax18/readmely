// Liveness for the container healthcheck. Touches no dependency, so the web
// app stays "healthy" while the API is down and the page shows that instead.
export const dynamic = "force-dynamic";

export function GET(): Response {
  return Response.json({ status: "ok" }, { headers: { "Cache-Control": "no-store" } });
}
