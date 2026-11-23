import { describe, expect, it } from "vitest";
import { loginErrorMessage } from "./login-errors";

// The codes contracts/core-public.yaml documents for the login redirect.
const CODES = [
  "access_denied",
  "invalid_state",
  "no_verified_email",
  "provider_conflict",
  "identity_in_use",
  "unsupported_email",
  "provider_error",
  "account_unavailable",
];

describe("loginErrorMessage", () => {
  it.each(CODES)("has a plain message for %s", (code) => {
    const text = loginErrorMessage(code);
    expect(text).toBeTruthy();
    expect(text).not.toContain(code); // the code itself is never the message
  });

  it("never reveals whether an email belongs to another account", () => {
    for (const code of CODES) expect(loginErrorMessage(code)).not.toMatch(/@|exists|registered/i);
  });

  it.each([
    ["an unknown code", "made_up"],
    ["markup", "<img src=x onerror=alert(1)>"],
    ["an empty string", ""],
    ["a prototype key", "__proto__"],
    ["an inherited key", "constructor"],
    ["another inherited key", "toString"],
    ["undefined", undefined],
    ["a repeated parameter", ["access_denied", "provider_error"]],
  ])("shows nothing for %s", (_name, value) => {
    expect(loginErrorMessage(value as string | undefined)).toBeNull();
  });
});
