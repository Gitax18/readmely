import type { Metadata } from "next";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { AccountView } from "@/components/account-view";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { UnavailableScreen } from "@/components/layout/unavailable";
import {
  fetchCredits,
  fetchGitHubAccess,
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
    return <UnavailableScreen title="Account unavailable" />;
  }

  const [sessions, identities, providers, credits, githubAccess] = await Promise.all([
    fetchSessions(env.apiPublicUrl, token),
    fetchIdentities(env.apiPublicUrl, token),
    fetchProviders(env.apiPublicUrl),
    fetchCredits(env.apiPublicUrl, token),
    fetchGitHubAccess(env.apiPublicUrl, token),
  ]);
  return (
    <AppShell
      active="account"
      email={session.me.user.email}
      credits={credits}
      csrf={session.me.csrf_token}
    >
      <PageHeader title="Account" description="Your details, connected accounts and devices." />
      <AccountView
        me={session.me}
        initialSessions={sessions ?? []}
        initialIdentities={identities ?? []}
        providers={providers}
        initialGitHubAccess={githubAccess}
        notice={loginErrorMessage(error)}
      />
    </AppShell>
  );
}
