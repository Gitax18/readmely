import type { Metadata } from "next";
import Link from "next/link";
import { StatusPanel } from "@/components/status-panel";
import { Wordmark } from "@/components/ui/logo";
import { ThemeToggle } from "@/components/ui/theme-toggle";
import { SiteFooter } from "@/components/layout/site-footer";
import { loadEnv } from "@/lib/env";
import { fetchStatus } from "@/lib/status-client";
import { toView } from "@/lib/status-view";

export const metadata: Metadata = {
  title: "Readmely status",
  description: "Current status of the Readmely services.",
};

// Status must be live: render on every request, never from a build-time cache.
export const dynamic = "force-dynamic";

export default async function StatusPage() {
  const env = loadEnv();
  const result = await fetchStatus(env.apiInternalUrl);
  const view = toView(result, env.statusDetail);

  return (
    <div className="flex min-h-screen flex-col">
      <header className="mx-auto flex h-[68px] w-full max-w-[1100px] items-center justify-between px-4 sm:px-6">
        <Wordmark />
        <ThemeToggle />
      </header>
      <main
        id="main"
        className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-8 px-4 py-10 sm:px-6"
      >
        <header className="animate-fade-up">
          <h1 className="text-2xl font-semibold tracking-tight sm:text-[28px]">Readmely status</h1>
          <p className="text-muted-foreground mt-1 text-sm">Reload the page to refresh.</p>
          <p className="mt-3 text-sm">
            <Link
              href="/dashboard"
              className="text-accent font-medium underline-offset-4 hover:underline"
            >
              Sign in or open your account
            </Link>
          </p>
        </header>
        <StatusPanel view={view} />
      </main>
      <SiteFooter compact />
    </div>
  );
}
