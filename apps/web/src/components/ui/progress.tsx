import { cn } from "@/lib/utils";

/** A labelled progress bar; the label is what a screen reader announces. */
export function Progress({
  value,
  label,
  className,
}: {
  value: number;
  label: string;
  className?: string;
}) {
  const v = Math.max(0, Math.min(100, Math.round(value)));
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={v}
      className={cn("h-2 w-full overflow-hidden rounded-full bg-border", className)}
    >
      <div className="h-full bg-foreground transition-all" style={{ width: `${v}%` }} />
    </div>
  );
}
