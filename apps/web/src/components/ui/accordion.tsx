import { ChevronDown } from "lucide-react";
import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * A native disclosure list. `<details>` gives keyboard and screen-reader behaviour for free, works
 * before any script loads, and puts no inline style in the markup (which the content security
 * policy would refuse). Items that share a `name` open one at a time.
 */
export function Accordion({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("border-border border-t", className)} {...props} />;
}

export function AccordionItem({
  title,
  name,
  defaultOpen,
  className,
  children,
}: {
  title: React.ReactNode;
  name?: string;
  defaultOpen?: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <details
      {...(name ? { name } : {})}
      {...(defaultOpen ? { open: true } : {})}
      className={cn("group border-border border-b", className)}
    >
      <summary
        className={cn(
          "hover:text-accent flex cursor-pointer list-none items-center justify-between gap-4 py-5 text-left text-sm font-semibold transition-colors",
          "[&::-webkit-details-marker]:hidden",
        )}
      >
        {title}
        <ChevronDown
          aria-hidden
          className="text-faint size-4 shrink-0 transition-transform duration-300 group-open:rotate-180"
        />
      </summary>
      <div className="animate-fade-in text-muted-foreground max-w-prose pb-5 text-[13px] leading-relaxed">
        {children}
      </div>
    </details>
  );
}
