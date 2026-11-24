"use client";

import { AnimatePresence, motion } from "motion/react";
import { ArrowRight, Menu, X } from "lucide-react";
import Link from "next/link";
import * as React from "react";
import { buttonVariants } from "@/components/ui/button";
import { Wordmark } from "@/components/ui/logo";
import { ThemeToggle } from "@/components/ui/theme-toggle";
import { cn } from "@/lib/utils";

const LINKS = [
  { href: "#how", label: "How it works" },
  { href: "#sample", label: "Sample" },
  { href: "#pricing", label: "Pricing" },
];

/** The public top bar: anchors on the page, theme, and the two ways in (or "Open dashboard"). */
export function LandingHeader({ signedIn }: { signedIn: boolean }) {
  const [open, setOpen] = React.useState(false);
  const [scrolled, setScrolled] = React.useState(false);
  React.useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 8);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <header
      className={cn(
        "sticky top-0 z-40 border-b transition-[background-color,border-color,backdrop-filter] duration-300",
        scrolled
          ? "bg-background/85 border-border backdrop-blur-md"
          : "border-transparent bg-transparent",
      )}
    >
      <div className="relative mx-auto flex h-[68px] w-full max-w-[1180px] items-center justify-between px-4 sm:px-6">
        <Wordmark />
        <nav
          aria-label="Page sections"
          className="absolute left-1/2 hidden -translate-x-1/2 items-center gap-9 md:flex"
        >
          {LINKS.map((l) => (
            <a
              key={l.href}
              href={l.href}
              className="text-muted-foreground hover:text-foreground text-[13px] font-semibold transition-colors"
            >
              {l.label}
            </a>
          ))}
        </nav>
        <div className="flex items-center gap-2">
          <ThemeToggle />
          {signedIn ? (
            <Link href="/dashboard" className={buttonVariants({ variant: "primary" })}>
              Open dashboard <ArrowRight aria-hidden />
            </Link>
          ) : (
            <>
              <Link
                href="/login"
                className={cn(buttonVariants({ variant: "secondary" }), "hidden sm:inline-flex")}
              >
                Sign in
              </Link>
              <Link
                href="/login"
                className={cn(buttonVariants({ variant: "primary" }), "hidden sm:inline-flex")}
              >
                Get started <ArrowRight aria-hidden />
              </Link>
            </>
          )}
          <button
            type="button"
            aria-label={open ? "Close menu" : "Open menu"}
            aria-expanded={open}
            aria-controls="landing-menu"
            onClick={() => setOpen((o) => !o)}
            className="text-muted-foreground hover:bg-surface-2 grid size-9 place-items-center rounded-control md:hidden"
          >
            {open ? <X className="size-5" aria-hidden /> : <Menu className="size-5" aria-hidden />}
          </button>
        </div>

        <AnimatePresence>
          {open ? (
            <motion.nav
              id="landing-menu"
              aria-label="Page sections"
              initial={{ opacity: 0, y: -8 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -8 }}
              transition={{ duration: 0.18 }}
              className="bg-card border-border shadow-lift absolute inset-x-0 top-full flex flex-col gap-1 border-b p-3 md:hidden"
            >
              {LINKS.map((l) => (
                <a
                  key={l.href}
                  href={l.href}
                  onClick={() => setOpen(false)}
                  className="text-muted-foreground hover:bg-surface-2 rounded-control px-3 py-2.5 text-sm font-semibold"
                >
                  {l.label}
                </a>
              ))}
              {!signedIn ? (
                <div className="mt-2 flex gap-2">
                  <Link
                    href="/login"
                    className={cn(buttonVariants({ variant: "secondary" }), "flex-1")}
                  >
                    Sign in
                  </Link>
                  <Link
                    href="/login"
                    className={cn(buttonVariants({ variant: "primary" }), "flex-1")}
                  >
                    Get started
                  </Link>
                </div>
              ) : null}
            </motion.nav>
          ) : null}
        </AnimatePresence>
      </div>
    </header>
  );
}
