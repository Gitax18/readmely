// Runs once when the server starts: a bad environment stops the process
// instead of failing on the first request.
export async function register(): Promise<void> {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    const { loadEnv } = await import("./lib/env");
    loadEnv();
  }
}
