import { cva, type VariantProps } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * One definition of what a button looks like. `Button` uses it for real buttons; links that look
 * like buttons call `buttonVariants(...)` for their class name.
 */
export const buttonVariants = cva(
  [
    "inline-flex shrink-0 items-center justify-center gap-2 rounded-control text-[13px] font-semibold whitespace-nowrap",
    "transition-[transform,background-color,border-color,color,opacity] duration-200",
    "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
    "disabled:cursor-not-allowed disabled:opacity-50 aria-disabled:cursor-not-allowed aria-disabled:opacity-50",
    "enabled:active:translate-y-px [&_svg]:size-4 [&_svg]:shrink-0",
  ],
  {
    variants: {
      variant: {
        primary: "border border-transparent bg-foreground text-background hover:opacity-90",
        accent: "border border-transparent bg-accent text-accent-foreground hover:bg-accent-hover",
        secondary:
          "border border-border bg-card text-card-foreground hover:border-border-strong hover:bg-surface-2",
        danger: "border border-bad/60 bg-transparent text-bad hover:bg-bad/10",
        ghost:
          "border border-transparent text-muted-foreground hover:bg-surface-2 hover:text-foreground",
        link: "h-auto border-0 px-0 text-accent underline-offset-4 hover:underline",
        light: "border border-transparent bg-white text-[#142126] hover:bg-white/90",
      },
      size: {
        sm: "h-9 px-3",
        md: "h-10 px-4",
        lg: "h-12 px-6 text-sm",
        icon: "size-9 px-0",
      },
    },
    defaultVariants: { variant: "primary", size: "md" },
  },
);

export type ButtonProps = React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    /** Shows a spinner and blocks clicks while work is running. The label stays as it is. */
    busy?: boolean;
  };

export function Button({
  className,
  variant,
  size,
  busy = false,
  type = "button",
  disabled,
  children,
  ...props
}: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    >
      {busy ? <Loader2 className="animate-spin" aria-hidden /> : null}
      {children}
    </button>
  );
}
