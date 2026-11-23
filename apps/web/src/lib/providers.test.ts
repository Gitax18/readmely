import { describe, expect, it } from "vitest";
import {
  isProviderAuthorizationUrl,
  isSiteRelativePath,
  PROVIDER_LABELS,
  PROVIDERS,
} from "./providers";

describe("isProviderAuthorizationUrl", () => {
  it.each([
    "https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y",
    "https://accounts.google.com/o/oauth2/v2/auth",
    "https://github.com/login/oauth/authorize?client_id=x&state=y",
  ])("accepts %s", (url) => expect(isProviderAuthorizationUrl(url)).toBe(true));

  it.each([
    ["another site", "https://evil.example/o/oauth2/v2/auth"],
    ["a lookalike host", "https://accounts.google.com.evil.example/o/oauth2/v2/auth"],
    ["a prefix lookalike", "https://accounts.google.com.attacker.io/o/oauth2/v2/auth?x"],
    ["userinfo", "https://accounts.google.com@evil.example/o/oauth2/v2/auth"],
    ["credentials", "https://user:pw@github.com/login/oauth/authorize"],
    ["http", "http://github.com/login/oauth/authorize"],
    ["another port", "https://github.com:8443/login/oauth/authorize"],
    ["another path", "https://github.com/login/oauth/access_token"],
    ["a longer path", "https://github.com/login/oauth/authorize/x"],
    ["a fragment", "https://github.com/login/oauth/authorize?x=1#frag"],
    ["javascript", "javascript:alert(1)"],
    ["data", "data:text/html,x"],
    ["a relative path", "/login/oauth/authorize"],
    ["empty", ""],
    ["not a URL", "not a url"],
    ["enormous", `https://github.com/login/oauth/authorize?${"a".repeat(3000)}`],
  ])("refuses %s", (_name, url) => expect(isProviderAuthorizationUrl(url)).toBe(false));
});

describe("isSiteRelativePath", () => {
  it.each(["/account", "/login?error=access_denied", "/a/b-c_d.e~f"])("accepts %s", (p) =>
    expect(isSiteRelativePath(p)).toBe(true),
  );
  it.each([
    "",
    "account",
    "//evil.example",
    "/\\evil.example",
    "https://evil.example/",
    "/a/../b",
    "/a b",
    "/a\nb",
    "/a\u0000",
    "/a#frag",
    "/<script>",
    `/${"a".repeat(3000)}`,
  ])("refuses %j", (p) => expect(isSiteRelativePath(p)).toBe(false));
});

describe("the provider list", () => {
  it("has a label for every provider", () => {
    expect(PROVIDERS.map((p) => PROVIDER_LABELS[p])).toEqual(["Google", "GitHub"]);
  });
});
