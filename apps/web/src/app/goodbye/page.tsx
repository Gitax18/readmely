import type { Metadata } from "next";
import Link from "next/link";
import { CenteredShell } from "@/components/layout/app-shell";
import { buttonVariants } from "@/components/ui/button";
import { Wordmark } from "@/components/ui/logo";

export const metadata: Metadata = { title: "Account deleted · Readmely" };

export default function GoodbyePage() {
  return (
    <CenteredShell>
      <Wordmark />
      <div className="animate-fade-up flex flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">Your account was deleted</h1>
        <p className="text-muted-foreground text-sm">
          Your projects and everything generated for them are gone, and you are signed out on every
          device. We sent a confirmation to your email address.
        </p>
        <p className="text-sm">Changed your mind? Sign in again to start a new account.</p>
        <div>
          <Link href="/login" className={buttonVariants({ variant: "secondary" })}>
            Sign in again
          </Link>
        </div>
      </div>
    </CenteredShell>
  );
}
