import Link from "next/link";
import { AppNav, type NavKey } from "@/components/layout/app-nav";
import { UserMenu } from "@/components/layout/user-menu";
import { Wordmark } from "@/components/ui/logo";
import { ThemeToggle } from "@/components/ui/theme-toggle";
import { cn } from "@/lib/utils";

/** Under this many credits the pill turns amber; at zero it says so. */
export const LOW_CREDITS = 20;

/** The balance, always visible, so the cost of anything is never a surprise. */
export function CreditPill({ credits }: { credits: number }) {
  const empty = credits <= 0;
  const low = !empty && credits < LOW_CREDITS;
  return (
    <Link
      href="/usage"
      className={cn(
        "inline-flex h-9 items-center gap-2 rounded-control border px-3 text-xs font-semibold transition-colors",
        empty
          ? "border-bad/40 bg-bad/10 text-bad"
          : low
            ? "border-warn/40 bg-warn/10 text-warn"
            : "border-border bg-card text-foreground hover:bg-surface-2",
      )}
    >
      <span
        aria-hidden
        className={cn("size-1.5 rounded-full", empty ? "bg-bad" : low ? "bg-warn" : "bg-accent")}
      />
      {empty ? "Out of credits" : `${credits.toLocaleString("en-US")} credits`}
    </Link>
  );
}

/** The top bar of the signed-in app: the way between projects, usage and the account. */
export function AppHeader({
  active,
  email,
  credits,
  csrf,
}: {
  active: NavKey;
  email?: string | undefined;
  credits?: number | null | undefined;
  csrf?: string | undefined;
}) {
  return (
    <header className="bg-background/85 border-border sticky top-0 z-40 border-b backdrop-blur-md">
      <div className="relative mx-auto flex h-16 w-full max-w-[1100px] items-center gap-4 px-4 sm:px-6">
        <Wordmark href="/dashboard" />
        <div className="md:ml-4">
          <AppNav active={active} />
        </div>
        <div className="ml-auto flex items-center gap-2">
          {typeof credits === "number" ? <CreditPill credits={credits} /> : null}
          <ThemeToggle />
          {email ? <UserMenu email={email} {...(csrf ? { csrf } : {})} /> : null}
        </div>
      </div>
    </header>
  );
}
