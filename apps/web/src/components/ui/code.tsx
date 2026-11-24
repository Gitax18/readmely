import * as React from "react";
import { cn } from "@/lib/utils";

/** A file path, command or setting name, in monospace with a hairline border. */
export function Code({ className, ...props }: React.ComponentProps<"code">) {
  return (
    <code
      className={cn(
        "border-border bg-surface-2 rounded-md border px-1.5 py-0.5 text-xs break-all",
        className,
      )}
      {...props}
    />
  );
}

/** A block of code or Markdown source: dark, scrollable and focusable so it can be read by keyboard. */
export function CodeBlock({ className, ...props }: React.ComponentProps<"pre">) {
  return (
    <pre
      tabIndex={0}
      className={cn(
        "bg-code text-code-foreground scroll-thin overflow-auto rounded-lg p-4 text-xs leading-relaxed",
        className,
      )}
      {...props}
    />
  );
}
