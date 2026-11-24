import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { Wordmark } from "@/components/ui/logo";
import { ThemeToggle } from "@/components/ui/theme-toggle";

/** A slim top bar for public pages that are not the landing page (legal pages, status). */
export function PublicHeader({ signedIn = false }: { signedIn?: boolean }) {
  return (
    <header className="mx-auto flex h-[68px] w-full max-w-[1100px] items-center justify-between px-4 sm:px-6">
      <Wordmark />
      <div className="flex items-center gap-2">
        <ThemeToggle />
        <Link
          href={signedIn ? "/dashboard" : "/login"}
          className={buttonVariants({ variant: "secondary" })}
        >
          {signedIn ? "Open dashboard" : "Sign in"}
        </Link>
      </div>
    </header>
  );
}
