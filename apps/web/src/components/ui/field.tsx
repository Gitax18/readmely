import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * A form control with its visible label, an optional hint, and an error that is announced and
 * tied to the control. Pass the control as a function to receive the ids it must carry.
 */
export function Field({
  label,
  hint,
  error,
  className,
  children,
}: {
  label: React.ReactNode;
  hint?: React.ReactNode;
  error?: React.ReactNode;
  className?: string;
  children: (props: {
    id: string;
    "aria-describedby"?: string | undefined;
    "aria-invalid"?: true | undefined;
  }) => React.ReactNode;
}) {
  const id = React.useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <label htmlFor={id} className="text-[13px] font-semibold">
        {label}
      </label>
      {children({ id, "aria-describedby": describedBy, "aria-invalid": error ? true : undefined })}
      {hint ? (
        <p id={hintId} className="text-muted-foreground text-xs">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} role="alert" className="text-bad text-xs">
          {error}
        </p>
      ) : null}
    </div>
  );
}
