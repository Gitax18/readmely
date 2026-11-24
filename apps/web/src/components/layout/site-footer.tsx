import Link from "next/link";
import { GitHubMark } from "@/components/ui/brand-icons";
import { Wordmark } from "@/components/ui/logo";
import { cn } from "@/lib/utils";

export const REPO_URL = "https://github.com/Gitax18/readmely";

type Health = "operational" | "degraded" | "unavailable" | null;

const HEALTH_TEXT: Record<Exclude<Health, null>, { text: string; dot: string }> = {
  operational: { text: "All systems operational", dot: "bg-ok" },
  degraded: { text: "Some services are degraded", dot: "bg-warn" },
  unavailable: { text: "Service unavailable", dot: "bg-bad" },
};

/**
 * The foot of every page. `health` is the live status when the page has it; without it the line
 * is left out rather than claiming everything is fine.
 */
export function SiteFooter({
  health = null,
  compact = false,
  className,
}: {
  health?: Health;
  compact?: boolean;
  className?: string;
}) {
  const links = (
    <nav
      aria-label="Footer"
      className="text-muted-foreground flex flex-wrap items-center gap-x-6 gap-y-2 text-xs"
    >
      <Link href="/privacy" className="hover:text-foreground transition-colors">
        Privacy
      </Link>
      <Link href="/terms" className="hover:text-foreground transition-colors">
        Terms
      </Link>
      <Link href="/status" className="hover:text-foreground transition-colors">
        Status
      </Link>
      <a
        href={REPO_URL}
        target="_blank"
        rel="noreferrer noopener"
        className="hover:text-foreground inline-flex items-center gap-1.5 transition-colors"
      >
        <GitHubMark width={15} height={15} /> GitHub
      </a>
    </nav>
  );

  if (compact) {
    return (
      <footer className={cn("border-border mt-auto border-t", className)}>
        <div className="mx-auto flex w-full max-w-[1100px] flex-wrap items-center justify-between gap-3 px-4 py-5 sm:px-6">
          <span className="text-faint text-xs">© 2026 Readmely</span>
          {links}
        </div>
      </footer>
    );
  }

  const h = health ? HEALTH_TEXT[health] : null;
  return (
    <footer className={cn("mx-auto w-full max-w-[1100px] px-4 sm:px-6", className)}>
      <div className="flex flex-wrap items-center gap-x-8 gap-y-4 py-10">
        <Wordmark />
        <p className="text-faint text-xs">Documentation grounded in your code.</p>
        <div className="sm:ml-auto">{links}</div>
      </div>
      <div className="border-border text-faint flex flex-wrap items-center justify-between gap-2 border-t py-5 text-xs">
        <span>© 2026 Readmely</span>
        {h ? (
          <a
            href="/status"
            className="hover:text-foreground inline-flex items-center gap-2 transition-colors"
          >
            <span aria-hidden className={cn("size-1.5 rounded-full", h.dot)} />
            {h.text}
          </a>
        ) : null}
      </div>
    </footer>
  );
}
