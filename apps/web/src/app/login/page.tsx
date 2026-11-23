import type { Metadata } from "next";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { LoginForm } from "@/components/login-form";
import { fetchProviders, fetchSession, SESSION_COOKIE } from "@/lib/account-client";
import { loadApiPublicEnv } from "@/lib/env";
import { loginErrorMessage } from "@/lib/login-errors";

export const metadata: Metadata = { title: "Sign in · Readmely" };
export const dynamic = "force-dynamic";

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string | string[] }>;
}) {
  const env = loadApiPublicEnv();
  const { error } = await searchParams;
  const providerError = loginErrorMessage(error);
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  // Already signed in: nothing to do here. A failed "connect" lands on /login too, so the reason
  // is handed to the account page instead of being lost.
  if ((await fetchSession(env.apiPublicUrl, token)).kind === "signed_in") {
    redirect(providerError && typeof error === "string" ? `/account?error=${error}` : "/account");
  }
  const providers = await fetchProviders(env.apiPublicUrl);

  return (
    <main className="mx-auto flex max-w-md flex-col gap-6 p-6 sm:p-10">
      <h1 className="text-2xl font-semibold">Readmely</h1>
      <LoginForm providers={providers} providerError={providerError} />
    </main>
  );
}
