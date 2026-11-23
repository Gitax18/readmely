import type { Metadata } from "next";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { AccountView } from "@/components/account-view";
import { AppHeader } from "@/components/app-header";
import {
  fetchIdentities,
  fetchProviders,
  fetchSession,
  fetchSessions,
  SESSION_COOKIE,
} from "@/lib/account-client";
import { loadApiPublicEnv } from "@/lib/env";
import { loginErrorMessage } from "@/lib/login-errors";

export const metadata: Metadata = { title: "Account · Readmely" };
export const dynamic = "force-dynamic";

export default async function AccountPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string | string[] }>;
}) {
  const env = loadApiPublicEnv();
  const { error } = await searchParams;
  const token = (await cookies()).get(SESSION_COOKIE)?.value;

  // The check happens here, on the server, before any account markup is sent.
  const session = await fetchSession(env.apiPublicUrl, token);
  if (session.kind === "signed_out") redirect("/login");
  if (session.kind === "error" || token === undefined) {
    return (
      <main className="mx-auto flex max-w-md flex-col gap-4 p-6 sm:p-10">
        <h1 className="text-2xl font-semibold">Account unavailable</h1>
        <p role="alert" className="text-muted-foreground text-sm">
          We could not load your account just now. Reload the page in a moment.
        </p>
      </main>
    );
  }

  const [sessions, identities, providers] = await Promise.all([
    fetchSessions(env.apiPublicUrl, token),
    fetchIdentities(env.apiPublicUrl, token),
    fetchProviders(env.apiPublicUrl),
  ]);
  return (
    <main className="mx-auto flex max-w-2xl flex-col gap-6 p-6 sm:p-10">
      <AppHeader active="account" />
      <h1 className="text-2xl font-semibold">Account</h1>
      <AccountView
        me={session.me}
        initialSessions={sessions ?? []}
        initialIdentities={identities ?? []}
        providers={providers}
        notice={loginErrorMessage(error)}
      />
    </main>
  );
}
