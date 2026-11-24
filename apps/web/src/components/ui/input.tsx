import * as React from "react";
import { cn } from "@/lib/utils";

const fieldBase = [
  "w-full rounded-control border border-border bg-card px-3 text-sm text-card-foreground",
  "placeholder:text-faint transition-[border-color,box-shadow] duration-200",
  "hover:border-border-strong",
  "focus-visible:border-accent focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-accent/15",
  "aria-invalid:border-bad aria-invalid:focus-visible:ring-bad/15",
  "disabled:cursor-not-allowed disabled:opacity-50",
];

export function Input({ className, ...props }: React.ComponentProps<"input">) {
  return <input className={cn(fieldBase, "h-10", className)} {...props} />;
}

export function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return <textarea className={cn(fieldBase, "min-h-24 py-2.5", className)} {...props} />;
}
