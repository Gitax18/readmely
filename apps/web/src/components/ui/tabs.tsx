"use client";

import { motion } from "motion/react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * In-page tabs. Written here rather than taken from a library because every library tab list puts
 * a `style` attribute in the server markup, which the content security policy refuses. Follows the
 * ARIA tabs pattern: arrow keys, Home and End move between tabs, only the current tab is tabbable.
 */
type TabsContextValue = { value: string; set: (v: string) => void; id: string; order: string[] };
const TabsContext = React.createContext<TabsContextValue | null>(null);

function useTabs(): TabsContextValue {
  const ctx = React.useContext(TabsContext);
  if (!ctx) throw new Error("Tabs parts must be used inside <Tabs>");
  return ctx;
}

export function Tabs({
  defaultValue,
  order,
  children,
  className,
}: {
  defaultValue: string;
  /** The tab values in display order, used for arrow-key movement. */
  order: string[];
  children: React.ReactNode;
  className?: string;
}) {
  const [value, set] = React.useState(defaultValue);
  const id = React.useId();
  return (
    <TabsContext.Provider value={{ value, set, id, order }}>
      <div className={className}>{children}</div>
    </TabsContext.Provider>
  );
}

export function TabsList({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      role="tablist"
      className={cn("border-border scroll-thin flex gap-6 overflow-x-auto border-b", className)}
      {...props}
    />
  );
}

export function TabsTrigger({
  value,
  className,
  children,
}: {
  value: string;
  className?: string;
  children: React.ReactNode;
}) {
  const { value: current, set, id, order } = useTabs();
  const active = current === value;

  function onKeyDown(event: React.KeyboardEvent<HTMLButtonElement>) {
    const i = order.indexOf(value);
    const next =
      event.key === "ArrowRight"
        ? order[(i + 1) % order.length]
        : event.key === "ArrowLeft"
          ? order[(i - 1 + order.length) % order.length]
          : event.key === "Home"
            ? order[0]
            : event.key === "End"
              ? order[order.length - 1]
              : undefined;
    if (next === undefined) return;
    event.preventDefault();
    set(next);
    document.getElementById(`${id}-tab-${next}`)?.focus();
  }

  return (
    <button
      type="button"
      role="tab"
      id={`${id}-tab-${value}`}
      aria-selected={active}
      aria-controls={`${id}-panel-${value}`}
      tabIndex={active ? 0 : -1}
      data-state={active ? "active" : "inactive"}
      onClick={() => set(value)}
      onKeyDown={onKeyDown}
      className={cn(
        "text-muted-foreground relative flex h-11 shrink-0 items-center gap-2 text-[13px] font-semibold transition-colors",
        "hover:text-foreground data-[state=active]:text-foreground",
        "after:bg-accent after:absolute after:inset-x-0 after:-bottom-px after:h-0.5 after:origin-left after:scale-x-0 after:transition-transform after:duration-300",
        "data-[state=active]:after:scale-x-100",
        className,
      )}
    >
      {children}
    </button>
  );
}

/** Renders only while its tab is current, so inactive panels cost nothing. */
export function TabsContent({
  value,
  className,
  children,
}: {
  value: string;
  className?: string;
  children: React.ReactNode;
}) {
  const { value: current, id } = useTabs();
  if (current !== value) return null;
  return (
    <div
      role="tabpanel"
      id={`${id}-panel-${value}`}
      aria-labelledby={`${id}-tab-${value}`}
      className={cn("animate-fade-in", className)}
    >
      {children}
    </div>
  );
}

export type NavTab = { href: string; label: string; icon?: React.ReactNode; dot?: boolean };

/**
 * Tabs that are routes. The current one is read from the address, and a single underline slides
 * between tabs as the route changes. `base` is the tab that owns the bare project address.
 */
export function NavTabs({
  tabs,
  base,
  label,
  className,
}: {
  tabs: NavTab[];
  base: string;
  label: string;
  className?: string;
}) {
  const pathname = usePathname();
  const id = React.useId();
  return (
    <nav
      aria-label={label}
      className={cn("border-border scroll-thin overflow-x-auto border-b", className)}
    >
      <ul className="flex gap-6">
        {tabs.map((tab) => {
          const active = tab.href === base ? pathname === base : pathname.startsWith(tab.href);
          return (
            <li key={tab.href} className="relative shrink-0">
              <Link
                href={tab.href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "flex h-11 items-center gap-2 text-[13px] font-semibold transition-colors [&_svg]:size-4",
                  active ? "text-foreground" : "text-muted-foreground hover:text-foreground",
                )}
              >
                {tab.icon}
                {tab.label}
                {tab.dot ? (
                  <span
                    role="img"
                    aria-label="has content"
                    className="bg-accent size-1.5 rounded-full"
                  />
                ) : null}
              </Link>
              {active ? (
                <motion.span
                  layoutId={`${id}-tab-line`}
                  className="bg-accent absolute inset-x-0 -bottom-px h-0.5"
                  transition={{ type: "spring", stiffness: 500, damping: 40 }}
                />
              ) : null}
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
