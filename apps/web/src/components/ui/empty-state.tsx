import * as React from "react";
import { cn } from "@/lib/utils";

/** What belongs here, and the one next step. Used for a first visit and for lists with no rows. */
export function EmptyState({
  icon,
  title,
  children,
  action,
  className,
}: {
  icon?: React.ReactNode;
  title: string;
  children?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "border-border bg-card shadow-soft flex flex-col items-center gap-3 rounded-card border border-dashed px-6 py-14 text-center",
        className,
      )}
    >
      {icon ? (
        <div className="bg-accent-soft text-accent grid size-12 place-items-center rounded-xl [&>svg]:size-6">
          {icon}
        </div>
      ) : null}
      <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
      {children ? <p className="text-muted-foreground max-w-md text-sm">{children}</p> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}
