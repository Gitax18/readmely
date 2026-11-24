import Link from "next/link";
import { cn } from "@/lib/utils";

/** The mark: a navy tile with an R and a teal corner. Decorative; the wordmark names the product. */
export function BrandMark({ className }: { className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "bg-navy relative grid size-[30px] shrink-0 place-items-center overflow-hidden rounded-lg text-sm font-bold text-white",
        "shadow-[inset_0_0_0_1px_rgb(255_255_255/0.1)]",
        className,
      )}
    >
      <span className="absolute -right-2.5 -bottom-2.5 size-5 rounded-full bg-[#1ba99b]" />
      <span className="relative">R</span>
    </span>
  );
}

export function Wordmark({ href = "/", className }: { href?: string; className?: string }) {
  return (
    <Link
      href={href}
      aria-label="Readmely home"
      className={cn(
        "inline-flex items-center gap-2.5 text-[17px] font-bold tracking-tight",
        className,
      )}
    >
      <BrandMark />
      Readmely
    </Link>
  );
}
