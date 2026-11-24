"use client";

import { Moon, Sun } from "lucide-react";
import * as React from "react";
import { cn } from "@/lib/utils";

export const THEME_KEY = "readmely-theme";

function subscribe(onChange: () => void): () => void {
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => observer.disconnect();
}

/** The theme is the class on <html>, which the inline script in the layout sets before paint. */
function useIsDark(): boolean {
  return React.useSyncExternalStore(
    subscribe,
    () => document.documentElement.classList.contains("dark"),
    () => false,
  );
}

/** Switches between light and dark and remembers the choice in this browser. */
export function ThemeToggle({ className }: { className?: string }) {
  const dark = useIsDark();
  function toggle() {
    const next = !dark;
    document.documentElement.classList.toggle("dark", next);
    try {
      localStorage.setItem(THEME_KEY, next ? "dark" : "light");
    } catch {
      // A blocked store only means the choice is not remembered.
    }
  }
  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={dark ? "Use the light theme" : "Use the dark theme"}
      className={cn(
        "text-muted-foreground hover:bg-surface-2 hover:text-foreground grid size-9 place-items-center rounded-control transition-colors",
        className,
      )}
    >
      {dark ? (
        <Sun className="size-[18px]" aria-hidden />
      ) : (
        <Moon className="size-[18px]" aria-hidden />
      )}
    </button>
  );
}
