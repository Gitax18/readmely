"use client";

import { RadioGroup as RadioPrimitive } from "radix-ui";
import * as React from "react";
import { cn } from "@/lib/utils";

/** A group of choices drawn as selectable cards (README layouts, context-pack tools). */
export function RadioCardGroup({
  className,
  ...props
}: React.ComponentProps<typeof RadioPrimitive.Root>) {
  return <RadioPrimitive.Root className={cn("grid gap-3", className)} {...props} />;
}

export function RadioCard({
  title,
  description,
  hint,
  className,
  ...props
}: Omit<React.ComponentProps<typeof RadioPrimitive.Item>, "children"> & {
  title: React.ReactNode;
  description?: React.ReactNode;
  hint?: React.ReactNode;
}) {
  return (
    <RadioPrimitive.Item
      className={cn(
        "group border-border bg-card flex flex-col gap-1 rounded-xl border p-4 text-left transition-[border-color,box-shadow,background-color] duration-200",
        "hover:border-border-strong disabled:cursor-not-allowed disabled:opacity-50",
        "data-[state=checked]:border-accent data-[state=checked]:bg-accent-soft/50 data-[state=checked]:ring-4 data-[state=checked]:ring-accent/10",
        className,
      )}
      {...props}
    >
      <span className="flex items-center gap-2 text-sm font-semibold">
        <span
          aria-hidden
          className="border-border-strong group-data-[state=checked]:border-accent grid size-4 shrink-0 place-items-center rounded-full border"
        >
          <RadioPrimitive.Indicator className="bg-accent size-2 rounded-full" />
        </span>
        {title}
      </span>
      {description ? <span className="text-muted-foreground text-xs">{description}</span> : null}
      {hint ? <span className="text-faint font-mono text-[11px]">{hint}</span> : null}
    </RadioPrimitive.Item>
  );
}
