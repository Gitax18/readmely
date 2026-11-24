import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * The second, explicit step before something that cannot be undone: it replaces the button, says
 * exactly what will happen, and offers the destructive action and a way out. Not a modal.
 */
export function ConfirmPanel({
  label,
  className,
  children,
  actions,
}: {
  /** Names the group for assistive technology. */
  label: string;
  className?: string;
  children: React.ReactNode;
  actions: React.ReactNode;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className={cn(
        "animate-fade-in border-bad/30 bg-bad/5 flex flex-col gap-3 rounded-control border p-4",
        className,
      )}
    >
      <div className="text-sm">{children}</div>
      <div className="flex flex-wrap gap-2">{actions}</div>
    </div>
  );
}
