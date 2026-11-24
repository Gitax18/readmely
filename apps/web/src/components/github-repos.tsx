"use client";

import { ArrowRight, Lock, Search } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { GitHubMark } from "@/components/ui/brand-icons";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Message } from "@/components/ui/message";
import { Skeleton } from "@/components/ui/skeleton";
import type { GitHubAccess } from "@/lib/account-schema";
import { createAccountApi, type AccountApi } from "@/lib/browser-api";
import type { GitHubRepository } from "@/lib/project-schema";
import { formatSize, projectFailureText } from "@/lib/project-text";
import type { ProjectsApi } from "@/lib/projects-api";

type State =
  | { kind: "loading" }
  | { kind: "off" }
  | { kind: "connect" }
  | { kind: "allow" }
  | { kind: "error"; text: string }
  | { kind: "list"; repos: GitHubRepository[] };

/**
 * The person's own GitHub repositories, private ones included, to pick from. It shows nothing when
 * the server has the feature off, asks them to connect GitHub or to allow access when that is the
 * missing step, and otherwise lists what GitHub says they can see.
 */
export function GitHubRepos({
  csrf,
  api,
  accountApi,
  adding,
  onAdd,
  onNavigate,
}: {
  csrf: string;
  api: ProjectsApi;
  accountApi?: AccountApi;
  /** The repository being added right now, as owner/name, or null. */
  adding: string | null;
  onAdd: (url: string, fullName: string) => void;
  /** Leaves for GitHub; replaceable in tests. */
  onNavigate?: (url: string) => void;
}) {
  const [account] = useState(() => accountApi ?? createAccountApi());
  const [state, setState] = useState<State>({ kind: "loading" });
  const [query, setQuery] = useState("");
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    void (async () => {
      const access = await account.getGitHubAccess();
      if (!live) return;
      if (!access.ok) return setState({ kind: "off" });
      const a: GitHubAccess = access.access;
      if (!a.enabled) return setState({ kind: "off" });
      if (!a.connected) return setState({ kind: "connect" });
      if (!a.granted) return setState({ kind: "allow" });
      const repos = await api.githubRepos();
      if (!live) return;
      if (repos.ok) setState({ kind: "list", repos: repos.repositories });
      else if (repos.kind === "conflict") setState({ kind: "allow" });
      else setState({ kind: "error", text: projectFailureText(repos) });
    })();
    return () => {
      live = false;
    };
  }, [account, api]);

  async function allow() {
    if (starting) return;
    setStarting(true);
    setStartError(null);
    const res = await account.startGitHubAccess(csrf);
    if (!res.ok) {
      setStarting(false);
      setStartError("We could not start that just now. Try again in a moment.");
      return;
    }
    (onNavigate ?? ((url: string) => window.location.assign(url)))(res.url);
  }

  const shown = useMemo(() => {
    if (state.kind !== "list") return [];
    const q = query.trim().toLowerCase();
    return q === ""
      ? state.repos
      : state.repos.filter((r) =>
          `${r.owner}/${r.name} ${r.description}`.toLowerCase().includes(q),
        );
  }, [state, query]);

  if (state.kind === "off") return null;

  return (
    <Card className="animate-fade-up">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          <GitHubMark className="size-5" /> Your GitHub repositories
        </CardTitle>
        <CardDescription>
          Pick one of your own, including private repositories, instead of pasting an address.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {state.kind === "loading" ? (
          <div role="status" aria-label="Loading your repositories" className="flex flex-col gap-2">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        ) : null}

        {state.kind === "connect" ? (
          <div className="flex flex-col items-start gap-3">
            <p className="text-muted-foreground text-sm">
              Connect your GitHub account to choose from your repositories.
            </p>
            <Link
              href="/account"
              className="text-accent text-sm font-semibold underline underline-offset-2"
            >
              Connect GitHub on your account page
            </Link>
          </div>
        ) : null}

        {state.kind === "allow" ? (
          <div className="flex flex-col items-start gap-3">
            <p className="text-muted-foreground max-w-prose text-sm">
              To list and read your private repositories, GitHub needs your permission. GitHub only
              offers a read-and-write permission for this, so that is what it will show; Readmely
              only ever reads. The access token is stored encrypted, and you can remove access from
              your account page at any time.
            </p>
            <Button variant="accent" busy={starting} onClick={() => void allow()}>
              <Lock aria-hidden /> Allow access to private repositories
            </Button>
            {startError ? <Message tone="error">{startError}</Message> : null}
          </div>
        ) : null}

        {state.kind === "error" ? <Message tone="error">{state.text}</Message> : null}

        {state.kind === "list" ? (
          <>
            <div className="relative">
              <Search
                className="text-faint pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
                aria-hidden
              />
              <Input
                type="search"
                aria-label="Search your repositories"
                placeholder="Search your repositories"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                className="pl-9"
              />
            </div>
            {shown.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {state.repos.length === 0
                  ? "GitHub shows no repositories for this account."
                  : "No repository matches that search."}
              </p>
            ) : (
              <ul
                aria-label="Your GitHub repositories"
                className="divide-border scroll-thin border-border max-h-[26rem] divide-y overflow-auto rounded-xl border"
              >
                {shown.map((r) => {
                  const full = `${r.owner}/${r.name}`;
                  const unavailable = r.added || r.too_large;
                  return (
                    <li key={full} className="flex items-center justify-between gap-3 p-3.5">
                      <div className="flex min-w-0 flex-col gap-0.5">
                        <span className="flex flex-wrap items-center gap-2 text-sm font-semibold">
                          <span className="break-all">{full}</span>
                          {r.private ? (
                            <Badge tone="neutral">
                              <Lock className="size-3" aria-hidden /> Private
                            </Badge>
                          ) : null}
                        </span>
                        {r.description ? (
                          <span className="text-muted-foreground line-clamp-1 text-xs">
                            {r.description}
                          </span>
                        ) : null}
                        <span className="text-faint text-xs">{formatSize(r.size_kb)}</span>
                      </div>
                      {r.added ? (
                        <Badge tone="accent">Added</Badge>
                      ) : r.too_large ? (
                        <Badge tone="warn">Too large</Badge>
                      ) : (
                        <Button
                          variant="secondary"
                          size="sm"
                          busy={adding === full}
                          disabled={adding !== null || unavailable}
                          aria-label={`Add ${full}`}
                          onClick={() => onAdd(`https://github.com/${full}`, full)}
                        >
                          Add <ArrowRight aria-hidden />
                        </Button>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}
