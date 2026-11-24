"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * A horizontal bar filled to `percent`. The width is written by script after the page loads rather
 * than in the markup, because the content security policy refuses inline styles; the CSS
 * transition then animates every change, including the first.
 */
export function Bar({
  percent,
  className,
  barClassName,
}: {
  percent: number;
  className?: string;
  barClassName?: string;
}) {
  const ref = React.useRef<HTMLDivElement>(null);
  const clamped = Math.max(0, Math.min(100, percent));
  React.useEffect(() => {
    if (ref.current) ref.current.style.width = `${clamped}%`;
  }, [clamped]);
  return (
    <div className={cn("bg-border h-2 w-full overflow-hidden rounded-full", className)}>
      <div
        ref={ref}
        className={cn(
          "bg-accent h-full w-0 rounded-full transition-[width] duration-700 ease-out",
          barClassName,
        )}
      />
    </div>
  );
}
