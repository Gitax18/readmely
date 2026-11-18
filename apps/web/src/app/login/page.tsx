import type { Metadata } from "next";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { LoginForm } from "@/components/login-form";
import { fetchSession, SESSION_COOKIE } from "@/lib/account-client";
import { loadApiPublicEnv } from "@/lib/env";

export const metadata: Metadata = { title: "Sign in · Readmely" };
export const dynamic = "force-dynamic";

export default async function LoginPage() {
  const env = loadApiPublicEnv();
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  // Already signed in: nothing to do here.
  if ((await fetchSession(env.apiPublicUrl, token)).kind === "signed_in") redirect("/account");

  return (
    <main className="mx-auto flex max-w-md flex-col gap-6 p-6 sm:p-10">
      <h1 className="text-2xl font-semibold">Readmely</h1>
      <LoginForm />
    </main>
  );
}
