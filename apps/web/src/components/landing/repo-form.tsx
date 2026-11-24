"use client";

import { ArrowRight, GitBranch } from "lucide-react";
import { useRouter } from "next/navigation";
import * as React from "react";
import { Button } from "@/components/ui/button";
import { GITHUB_REPO, INVALID_REPO } from "@/lib/github-url";
import { cn } from "@/lib/utils";

/**
 * The hero input. A valid address goes to sign-in with the address kept, and on to creating that
 * project afterwards; a signed-in visitor goes straight to creating it.
 */
export function RepoForm({ signedIn }: { signedIn: boolean }) {
  const router = useRouter();
  const [url, setUrl] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    const value = url.trim();
    if (!GITHUB_REPO.test(value)) {
      setError(INVALID_REPO);
      return;
    }
    setError(null);
    const repo = encodeURIComponent(value);
    router.push(signedIn ? `/projects/new?repo=${repo}` : `/login?repo=${repo}`);
  }

  return (
    <form onSubmit={submit} noValidate className="mx-auto w-full max-w-[660px] text-left">
      <label htmlFor="hero-repo" className="sr-only">
        GitHub repository address
      </label>
      <div
        className={cn(
          "bg-card shadow-soft flex flex-col gap-2 rounded-[10px] border p-2 transition-[border-color,box-shadow] duration-200 sm:h-[58px] sm:flex-row sm:items-center sm:gap-3 sm:pl-4",
          "focus-within:border-accent focus-within:ring-4 focus-within:ring-accent/15",
          error ? "border-bad" : "border-border-strong",
        )}
      >
        <GitBranch className="text-faint hidden size-[18px] shrink-0 sm:block" aria-hidden />
        <input
          id="hero-repo"
          type="url"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          placeholder="https://github.com/owner/repo"
          value={url}
          onChange={(e) => {
            setUrl(e.target.value);
            if (error) setError(null);
          }}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? "hero-repo-error" : undefined}
          className="placeholder:text-faint min-w-0 flex-1 bg-transparent px-2 py-2 font-mono text-[13px] outline-none sm:p-0"
        />
        <Button type="submit" variant="accent" size="lg" className="sm:h-[46px]">
          Generate <ArrowRight aria-hidden />
        </Button>
      </div>
      {error ? (
        <p id="hero-repo-error" role="alert" className="text-bad mt-2 px-1 text-xs">
          {error}
        </p>
      ) : null}
    </form>
  );
}
