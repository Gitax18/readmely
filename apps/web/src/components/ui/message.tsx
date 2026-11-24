import { AlertCircle, CheckCircle2, Info } from "lucide-react";
import * as React from "react";
import { cn } from "@/lib/utils";

type Tone = "error" | "success" | "info" | "warn";

const tones: Record<Tone, { box: string; icon: React.ReactNode }> = {
  error: { box: "border-bad/30 bg-bad/10 text-bad", icon: <AlertCircle /> },
  success: { box: "border-ok/30 bg-ok/10 text-ok", icon: <CheckCircle2 /> },
  warn: { box: "border-warn/30 bg-warn/10 text-warn", icon: <AlertCircle /> },
  info: { box: "border-border bg-surface-2 text-muted-foreground", icon: <Info /> },
};

/**
 * A plain-language notice under the control that caused it. Errors are announced at once
 * (`role="alert"`); the others politely. The icon means it is never colour alone.
 */
export function Message({
  tone = "info",
  className,
  children,
  ...props
}: React.ComponentProps<"div"> & { tone?: Tone }) {
  const t = tones[tone];
  return (
    <div
      role={tone === "error" ? "alert" : "status"}
      className={cn(
        "animate-fade-in flex items-start gap-2.5 rounded-control border px-3 py-2.5 text-sm [&>svg]:mt-0.5 [&>svg]:size-4 [&>svg]:shrink-0",
        t.box,
        className,
      )}
      {...props}
    >
      {React.cloneElement(t.icon as React.ReactElement<{ "aria-hidden": boolean }>, {
        "aria-hidden": true,
      })}
      <div className="min-w-0 break-words">{children}</div>
    </div>
  );
}
