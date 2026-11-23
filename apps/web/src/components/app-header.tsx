import Link from "next/link";

type Active = "projects" | "account";

/** The top bar of the signed-in app: the way between projects and the account. */
export function AppHeader({ active, email }: { active: Active; email?: string }) {
  const link = (href: string, label: string, key: Active) => (
    <Link
      href={href}
      aria-current={active === key ? "page" : undefined}
      className={
        active === key ? "text-sm font-medium underline" : "text-muted-foreground text-sm underline"
      }
    >
      {label}
    </Link>
  );
  return (
    <header className="flex flex-wrap items-center justify-between gap-3 border-b border-border pb-4">
      <div className="flex items-center gap-5">
        <Link href="/dashboard" className="text-lg font-semibold">
          Readmely
        </Link>
        <nav aria-label="Main" className="flex items-center gap-4">
          {link("/dashboard", "Projects", "projects")}
          {link("/account", "Account", "account")}
        </nav>
      </div>
      {email ? <span className="text-muted-foreground text-xs break-all">{email}</span> : null}
    </header>
  );
}
