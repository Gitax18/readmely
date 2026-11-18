import type { Me, Session } from "@/lib/account-schema";

export const CSRF = "ab".repeat(32);

export function me(overrides: Partial<Me["user"]> = {}): Me {
  return {
    user: {
      id: "01900000-0000-7000-8000-000000000001",
      email: "ada@example.com",
      display_name: "",
      created_at: "2026-11-18T09:30:00Z",
      ...overrides,
    },
    csrf_token: CSRF,
  };
}

export function session(overrides: Partial<Session> = {}): Session {
  return {
    id: "01900000-0000-7000-8000-000000000401",
    created_at: "2026-11-18T09:30:00Z",
    last_seen_at: "2026-11-18T10:02:00Z",
    device: "Safari on macOS",
    current: true,
    ...overrides,
  };
}

export function json(body: unknown, init: ResponseInit = {}): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
    ...init,
  });
}
