"use client";

import { AnimatePresence, motion } from "motion/react";
import { Menu, X } from "lucide-react";
import Link from "next/link";
import * as React from "react";
import { cn } from "@/lib/utils";

export type NavKey = "projects" | "usage" | "account";

const ITEMS: { key: NavKey; href: string; label: string }[] = [
  { key: "projects", href: "/dashboard", label: "Projects" },
  { key: "usage", href: "/usage", label: "Usage" },
  { key: "account", href: "/account", label: "Account" },
];

/** The main links: a row on wide screens, a drop-down panel behind a menu button on phones. */
export function AppNav({ active }: { active: NavKey }) {
  const [open, setOpen] = React.useState(false);
  const id = React.useId();
  return (
    <>
      <nav aria-label="Main" className="hidden items-center gap-1 md:flex">
        {ITEMS.map((item) => (
          <Link
            key={item.key}
            href={item.href}
            aria-current={active === item.key ? "page" : undefined}
            className={cn(
              "rounded-control px-3 py-1.5 text-[13px] font-semibold transition-colors",
              active === item.key
                ? "bg-surface-2 text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {item.label}
          </Link>
        ))}
      </nav>

      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        aria-label={open ? "Close menu" : "Open menu"}
        onClick={() => setOpen((o) => !o)}
        className="text-muted-foreground hover:bg-surface-2 grid size-9 place-items-center rounded-control md:hidden"
      >
        {open ? <X className="size-5" aria-hidden /> : <Menu className="size-5" aria-hidden />}
      </button>

      <AnimatePresence>
        {open ? (
          <motion.nav
            id={id}
            aria-label="Main"
            initial={{ opacity: 0, y: -8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            transition={{ duration: 0.18 }}
            className="bg-card border-border absolute inset-x-0 top-full flex flex-col gap-1 border-b p-3 shadow-lift md:hidden"
          >
            {ITEMS.map((item) => (
              <Link
                key={item.key}
                href={item.href}
                onClick={() => setOpen(false)}
                aria-current={active === item.key ? "page" : undefined}
                className={cn(
                  "rounded-control px-3 py-2.5 text-sm font-semibold",
                  active === item.key ? "bg-surface-2" : "text-muted-foreground",
                )}
              >
                {item.label}
              </Link>
            ))}
          </motion.nav>
        ) : null}
      </AnimatePresence>
    </>
  );
}
