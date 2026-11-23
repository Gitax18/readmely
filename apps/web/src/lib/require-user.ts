import "server-only";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { fetchCredits, fetchSession, SESSION_COOKIE } from "./account-client";
import { loadApiPublicEnv } from "./env";

/**
 * The check every signed-in page makes on the server before any markup is sent: no cookie or an
 * ended session goes to /login; an API that cannot be reached is "unavailable", never "signed out".
 */
export async function requireUser() {
  const env = loadApiPublicEnv();
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  const session = await fetchSession(env.apiPublicUrl, token);
  if (session.kind === "signed_out") redirect("/login");
  if (session.kind === "error" || token === undefined) return { unavailable: true as const };
  // The balance in the top bar is a convenience: if it cannot be read the page still works.
  const credits = await fetchCredits(env.apiPublicUrl, token);
  return { unavailable: false as const, env, token, me: session.me, credits };
}
