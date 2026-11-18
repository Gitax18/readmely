import { execFileSync } from "node:child_process";
import path from "node:path";
import { expect } from "@playwright/test";

const ROOT = path.resolve(import.meta.dirname, "../../..");
const COMPOSE = [
  "compose",
  "--env-file",
  process.env.ENV_FILE ?? "./.env",
  "-f",
  "infra/docker-compose.yml",
  "--profile",
  "app",
];

function compose(args: string[], env: NodeJS.ProcessEnv = process.env): string {
  return execFileSync("docker", [...COMPOSE, ...args], {
    cwd: ROOT,
    env,
    encoding: "utf8",
    maxBuffer: 16 * 1024 * 1024,
  });
}

/** A fresh address per call so tests never share a code, an account or a per-address limit. */
export function freshEmail(label: string): string {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;
  return `e2e-${label}-${id}@example.com`;
}

/**
 * The newest sign-in code emailed to an address, read from the API's development mailer log.
 * Waits for it: sending happens in the background after the request has been answered.
 */
export async function latestCode(email: string, after = 0): Promise<string> {
  let found: string | null = null;
  await expect
    .poll(
      () => {
        const out = compose(["logs", "--no-color", "--no-log-prefix", "--tail=2000", "api"]);
        const codes: string[] = [];
        for (const line of out.split("\n")) {
          if (!line.startsWith("{")) continue;
          try {
            const entry = JSON.parse(line) as { to?: string; text?: string };
            const m = entry.to === email ? /\b(\d{6})\b/.exec(entry.text ?? "") : null;
            if (m?.[1]) codes.push(m[1]);
          } catch {
            // not a JSON log line
          }
        }
        found = codes.length > after ? (codes[codes.length - 1] ?? null) : null;
        return found;
      },
      {
        message: `no sign-in code was emailed to ${email}`,
        timeout: 15_000,
        intervals: [300, 500, 1000],
      },
    )
    .not.toBeNull();
  return found as unknown as string;
}

/**
 * Clears the API's rate-limit counters, so repeated local runs do not trip the per-client
 * limit (20 codes an hour). Needs REDIS_PASSWORD, which `make test-e2e-stack` provides.
 */
export function resetRateLimits(): void {
  const password = process.env.REDIS_PASSWORD;
  if (!password) return;
  const env = { ...process.env, REDISCLI_AUTH: password };
  const keys = compose(
    ["exec", "-T", "-e", "REDISCLI_AUTH", "redis", "redis-cli", "--scan", "--pattern", "rm:rl:*"],
    env,
  )
    .split("\n")
    .filter(Boolean);
  if (keys.length === 0) return;
  compose(["exec", "-T", "-e", "REDISCLI_AUTH", "redis", "redis-cli", "DEL", ...keys], env);
}
