import "server-only";
import { headers } from "next/headers";
import { loadApiPublicEnv } from "./env";

/**
 * The visitor's address, for the API calls this server makes while rendering a page. The API
 * limits requests per visitor; without this, every page render would count against one shared
 * address. Passed on only when a trusted proxy in front of this server wrote it (WEB_BEHIND_PROXY),
 * because otherwise anyone could forge it. Outside a request, such as in tests, nothing is added.
 */
export async function visitorHeaders(): Promise<Record<string, string>> {
  try {
    if (!loadApiPublicEnv().behindProxy) return {};
    const forwarded = (await headers()).get("x-forwarded-for");
    return forwarded ? { "x-forwarded-for": forwarded } : {};
  } catch {
    return {};
  }
}
