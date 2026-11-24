"use client";

import Link from "next/link";
import { Button, buttonVariants } from "@/components/ui/button";
import { Wordmark } from "@/components/ui/logo";

/** Shown when a page fails unexpectedly. The error itself is never displayed. */
export default function ErrorPage({ reset }: { error: Error; reset: () => void }) {
  return (
    <div className="mx-auto flex min-h-screen max-w-md flex-col justify-center gap-6 px-4 py-12">
      <Wordmark />
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">Something went wrong on our side</h1>
        <p className="text-muted-foreground text-sm">
          The page could not be shown. Nothing was lost. Try again, and if it keeps happening, check
          the status page.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <Button onClick={reset}>Try again</Button>
          <Link href="/status" className={buttonVariants({ variant: "ghost" })}>
            Status
          </Link>
        </div>
      </div>
    </div>
  );
}
