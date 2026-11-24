import { cn } from "@/lib/utils";

/** A grey placeholder shape with a slow sheen, shaped like the content that is loading. */
export function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      aria-hidden
      className={cn("bg-surface-2 bg-shimmer animate-shimmer rounded-control", className)}
      {...props}
    />
  );
}
