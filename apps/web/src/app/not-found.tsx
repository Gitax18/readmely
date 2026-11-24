import Link from "next/link";
import { CenteredShell } from "@/components/layout/app-shell";
import { buttonVariants } from "@/components/ui/button";
import { Wordmark } from "@/components/ui/logo";

export const metadata = { title: "Page not found · Readmely" };

export default function NotFound() {
  return (
    <CenteredShell>
      <Wordmark />
      <div className="animate-fade-up flex flex-col gap-4">
        <p className="text-accent font-mono text-sm">404</p>
        <h1 className="text-2xl font-semibold tracking-tight">This page does not exist</h1>
        <p className="text-muted-foreground text-sm">
          The address may be wrong, or the project may have been deleted.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <Link href="/dashboard" className={buttonVariants({ variant: "primary" })}>
            Go to your projects
          </Link>
          <Link href="/" className={buttonVariants({ variant: "ghost" })}>
            Home page
          </Link>
        </div>
      </div>
    </CenteredShell>
  );
}
