import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = { title: "Account deleted · Readmely" };

export default function GoodbyePage() {
  return (
    <main className="mx-auto flex max-w-md flex-col gap-4 p-6 sm:p-10">
      <h1 className="text-2xl font-semibold">Your account was deleted</h1>
      <p className="text-muted-foreground text-sm">
        Your projects and everything generated for them are gone, and you are signed out on every
        device. We sent a confirmation to your email address.
      </p>
      <p className="text-sm">
        Changed your mind?{" "}
        <Link href="/login" className="underline">
          Sign in again
        </Link>{" "}
        to start a new account.
      </p>
    </main>
  );
}
