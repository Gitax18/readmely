"use client";

import { ArrowRight, CheckCircle2, GitBranch, Search } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { GitHubRepos } from "@/components/github-repos";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Message } from "@/components/ui/message";
import type { AccountApi } from "@/lib/browser-api";
import { createProjectsApi, type ProjectsApi } from "@/lib/projects-api";
import type { RepositoryPreview } from "@/lib/project-schema";
import { formatSize, projectFailureText } from "@/lib/project-text";
import { ANALYSIS_COST, ANALYSIS_TYPICAL } from "@/lib/project-text";

export function NewProjectForm({
  csrf,
  api,
  accountApi,
  initialUrl = "",
}: {
  csrf: string;
  api?: ProjectsApi;
  accountApi?: AccountApi;
  /** An address carried over from the landing page; it is checked as soon as the form opens. */
  initialUrl?: string;
}) {
  const router = useRouter();
  const [client] = useState(() => api ?? createProjectsApi());
  const [url, setUrl] = useState(initialUrl);
  const [preview, setPreview] = useState<RepositoryPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [creating, setCreating] = useState(false);
  const [adding, setAdding] = useState<string | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const autoChecked = useRef(false);

  function fail(f: Parameters<typeof projectFailureText>[0], inList = false) {
    if (f.kind === "unauthenticated") {
      router.replace("/login");
      return;
    }
    (inList ? setListError : setError)(projectFailureText(f));
  }

  async function runCheck(value: string) {
    setBusy(true);
    setError(null);
    setPreview(null);
    const res = await client.preview(csrf, value);
    setBusy(false);
    if (res.ok) setPreview(res.repository);
    else fail(res);
  }

  function check(event: FormEvent) {
    event.preventDefault();
    if (busy || url.trim() === "") return;
    void runCheck(url.trim());
  }

  // A repository address from the landing page is looked up once, without a click.
  useEffect(() => {
    if (autoChecked.current || initialUrl === "") return;
    autoChecked.current = true;
    void runCheck(initialUrl);
    // Runs once on open; `runCheck` is re-created every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function createAndAnalyse(address: string = url.trim(), fromList: string | null = null) {
    if (busy) return;
    setBusy(true);
    setCreating(fromList === null);
    setAdding(fromList);
    setError(null);
    setListError(null);
    const created = await client.create(csrf, address);
    if (!created.ok) {
      setBusy(false);
      setCreating(false);
      setAdding(null);
      fail(created, fromList !== null);
      return;
    }
    // If starting the analysis fails the project still exists; its page offers to try again.
    await client.analyse(csrf, created.project.id);
    router.push(`/projects/${created.project.id}`);
  }

  return (
    <div className="flex flex-col gap-6">
      <Card className="animate-fade-up">
        <CardHeader>
          <CardTitle className="text-xl">Add a project</CardTitle>
          <CardDescription>
            Paste the address of a GitHub repository, for example https://github.com/owner/repo, or
            pick one of yours below.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <form onSubmit={check} className="flex flex-col gap-3">
            <Field
              label="GitHub repository address"
              hint="Up to 200 MB. Private repositories: choose them from the list below."
            >
              {(field) => (
                <div className="flex flex-col gap-2 sm:flex-row">
                  <div className="relative flex-1">
                    <GitBranch
                      className="text-faint pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
                      aria-hidden
                    />
                    <Input
                      {...field}
                      name="url"
                      type="text"
                      inputMode="url"
                      autoComplete="off"
                      spellCheck={false}
                      maxLength={300}
                      placeholder="https://github.com/owner/repo"
                      value={url}
                      onChange={(e) => {
                        setUrl(e.target.value);
                        setPreview(null);
                        setError(null);
                      }}
                      className="pl-9 font-mono text-[13px]"
                    />
                  </div>
                  <Button
                    type="submit"
                    variant="secondary"
                    busy={busy && !creating}
                    disabled={busy || url.trim() === ""}
                  >
                    {busy && !creating ? null : <Search aria-hidden />}
                    Check repository
                  </Button>
                </div>
              )}
            </Field>
          </form>

          {preview ? (
            <div
              role="group"
              aria-label="Repository found"
              className="animate-fade-up border-accent/30 bg-accent-soft/40 flex flex-col gap-4 rounded-xl border p-5"
            >
              <div className="text-accent flex items-center gap-2 text-xs font-bold tracking-[0.1em] uppercase">
                <CheckCircle2 className="size-4" aria-hidden /> Repository found
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-base font-semibold break-all">
                  {preview.owner}/{preview.name}
                </span>
                <span className="text-muted-foreground text-sm">
                  {preview.description || "No description."}
                </span>
                <span className="text-muted-foreground text-xs">
                  Default branch {preview.default_branch} · {formatSize(preview.size_kb)}
                </span>
              </div>
              <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                <Button
                  variant="accent"
                  size="lg"
                  busy={creating}
                  onClick={() => void createAndAnalyse()}
                  disabled={busy}
                >
                  Create and analyse <ArrowRight aria-hidden />
                </Button>
                <span className="text-muted-foreground text-xs">
                  Analysis holds {ANALYSIS_COST} credits and is usually charged about{" "}
                  {ANALYSIS_TYPICAL}.
                </span>
              </div>
            </div>
          ) : null}

          <div id="repo-error" aria-live="polite">
            {error ? <Message tone="error">{error}</Message> : null}
          </div>
        </CardContent>
      </Card>
      <GitHubRepos
        csrf={csrf}
        api={client}
        {...(accountApi ? { accountApi } : {})}
        adding={adding}
        onAdd={(address, full) => void createAndAnalyse(address, full)}
      />
      {listError ? <Message tone="error">{listError}</Message> : null}
    </div>
  );
}
