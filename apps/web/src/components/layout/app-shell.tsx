import { AppHeader } from "@/components/app-header";
import type { NavKey } from "@/components/layout/app-nav";
import { SiteFooter } from "@/components/layout/site-footer";
import { cn } from "@/lib/utils";

const WIDTHS = {
  /** Forms and short pages. */
  narrow: "max-w-[720px]",
  /** Lists and the account. */
  medium: "max-w-[860px]",
  /** The project workspace, where the README editor needs two panes. */
  wide: "max-w-[1100px]",
} as const;

export type ShellWidth = keyof typeof WIDTHS;

/** Everything a signed-in screen shares: skip link, top bar, a centred column, footer. */
export function AppShell({
  active,
  email,
  credits,
  csrf,
  width = "medium",
  className,
  children,
}: {
  active: NavKey;
  email?: string | undefined;
  credits?: number | null | undefined;
  csrf?: string | undefined;
  width?: ShellWidth;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen flex-col">
      <a
        href="#main"
        className="bg-foreground text-background sr-only z-50 rounded-control px-3 py-2 text-sm focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
      >
        Skip to content
      </a>
      <AppHeader active={active} email={email} credits={credits} csrf={csrf} />
      <main
        id="main"
        className={cn(
          "mx-auto flex w-full flex-1 flex-col gap-6 px-4 py-8 sm:px-6 sm:py-10",
          WIDTHS[width],
          className,
        )}
      >
        {children}
      </main>
      <SiteFooter compact />
    </div>
  );
}

/** A centred card on a quiet page, for screens with no top bar (sign in, goodbye, errors). */
export function CenteredShell({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className="flex min-h-screen flex-col">
      <main
        id="main"
        className={cn(
          "relative mx-auto flex w-full max-w-md flex-1 flex-col justify-center gap-6 px-4 py-12",
          className,
        )}
      >
        {children}
      </main>
      <SiteFooter compact />
    </div>
  );
}
