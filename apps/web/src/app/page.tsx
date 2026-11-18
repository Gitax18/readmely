import Link from "next/link";
import { StatusPanel } from "@/components/status-panel";
import { loadEnv } from "@/lib/env";
import { fetchStatus } from "@/lib/status-client";
import { toView } from "@/lib/status-view";

// Status must be live: render on every request, never from a build-time cache.
export const dynamic = "force-dynamic";

export default async function Home() {
  const env = loadEnv();
  const result = await fetchStatus(env.apiInternalUrl);
  const view = toView(result, env.statusDetail);

  return (
    <main className="mx-auto flex max-w-2xl flex-col gap-6 p-6 sm:p-10">
      <header>
        <h1 className="text-2xl font-semibold">Readmely status</h1>
        <p className="text-muted-foreground text-sm">Reload the page to refresh.</p>
        <p className="mt-2 text-sm">
          <Link href="/account" className="underline">
            Sign in or open your account
          </Link>
        </p>
      </header>
      <StatusPanel view={view} />
    </main>
  );
}
