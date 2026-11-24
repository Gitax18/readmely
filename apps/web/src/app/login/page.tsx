import type { Metadata } from "next";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { LoginForm } from "@/components/login-form";
import { CenteredShell } from "@/components/layout/app-shell";
import { Wordmark } from "@/components/ui/logo";
import { ThemeToggle } from "@/components/ui/theme-toggle";
import { fetchProviders, fetchSession, SESSION_COOKIE } from "@/lib/account-client";
import { loadApiPublicEnv } from "@/lib/env";
import { repoParam } from "@/lib/github-url";
import { loginErrorMessage } from "@/lib/login-errors";

export const metadata: Metadata = { title: "Sign in · Readmely" };
export const dynamic = "force-dynamic";

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string | string[]; repo?: string | string[] }>;
}) {
  const env = loadApiPublicEnv();
  const { error, repo: repoRaw } = await searchParams;
  const providerError = loginErrorMessage(error);
  const repo = repoParam(repoRaw);
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  // Already signed in: nothing to do here. A failed "connect" lands on /login too, so the reason
  // is handed to the account page instead of being lost.
  if ((await fetchSession(env.apiPublicUrl, token)).kind === "signed_in") {
    if (providerError && typeof error === "string") redirect(`/account?error=${error}`);
    redirect(repo ? `/projects/new?repo=${encodeURIComponent(repo)}` : "/dashboard");
  }
  const providers = await fetchProviders(env.apiPublicUrl);

  return (
    <CenteredShell>
      <div
        aria-hidden
        className="bg-glow pointer-events-none absolute top-0 left-1/2 -z-10 h-[420px] w-[640px] max-w-full -translate-x-1/2"
      />
      <div className="flex items-center justify-between">
        <Wordmark />
        <ThemeToggle />
      </div>
      <h1 className="sr-only">Sign in to Readmely</h1>
      {repo ? (
        <p className="text-muted-foreground -mb-2 text-center text-sm">
          Sign in to add <code className="text-foreground text-xs break-all">{repo}</code>
        </p>
      ) : null}
      <LoginForm providers={providers} providerError={providerError} repo={repo} />
    </CenteredShell>
  );
}
