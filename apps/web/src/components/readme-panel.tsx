"use client";

import {
  CheckCircle2,
  Download,
  FilePlus2,
  History,
  RotateCcw,
  Save,
  Sparkles,
  Wand2,
  XCircle,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { usePolling } from "@/components/projects-view";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CopyButton } from "@/components/ui/copy-button";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { MarkdownView } from "@/components/ui/markdown";
import { Message } from "@/components/ui/message";
import { Progress } from "@/components/ui/progress";
import { RadioCard, RadioCardGroup } from "@/components/ui/radio-cards";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import type { ProjectsApi } from "@/lib/projects-api";
import { projectFailureText, readmeFailureText, readmeStepLabel } from "@/lib/project-text";
import type { ReadmeState, ReadmeTemplate, ReadmeVersions } from "@/lib/readme-schema";
import {
  GENERATE_COST,
  SECTION_COST,
  SOURCE_LABELS,
  TEMPLATE_HINTS,
  TEMPLATE_LABELS,
  sectionHeadings,
} from "@/lib/readme-text";

type Load = { kind: "loading" } | { kind: "error" } | { kind: "ready"; state: ReadmeState };

function isWorking(s: ReadmeState): boolean {
  return s.job !== null && ["queued", "running"].includes(s.job.status);
}

/** The README: generate it, edit it with a live preview, check its health, rewrite a section, go back a version. */
export function ReadmePanel({
  api,
  projectId,
  csrf,
  repoName,
}: {
  api: ProjectsApi;
  projectId: string;
  csrf: string;
  repoName: string;
}) {
  const router = useRouter();
  const [load, setLoad] = useState<Load>({ kind: "loading" });
  const [draft, setDraft] = useState("");
  const [blank, setBlank] = useState(false);
  const [template, setTemplate] = useState<ReadmeTemplate>("standard");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [versions, setVersions] = useState<ReadmeVersions["versions"] | null>(null);
  const [rewrite, setRewrite] = useState<{ heading: string; instruction: string } | null>(null);
  // On a phone the editor and the preview take turns; on a wide screen both show.
  const [pane, setPane] = useState<"edit" | "preview">("edit");

  function adopt(state: ReadmeState) {
    setLoad({ kind: "ready", state });
    setDraft(state.readme?.content ?? "");
  }

  useEffect(() => {
    let live = true;
    void api.readme(projectId).then((res) => {
      if (!live) return;
      if (res.ok) adopt(res.state);
      else setLoad({ kind: "error" });
    });
    return () => {
      live = false;
    };
  }, [api, projectId]);

  const state = load.kind === "ready" ? load.state : null;
  const working = state !== null && isWorking(state);

  // While a job runs, poll; when it settles take the new text and refresh the credit balance.
  usePolling(working, async () => {
    const res = await api.readme(projectId);
    if (!res.ok) return;
    if (isWorking(res.state)) {
      setLoad({ kind: "ready", state: res.state });
      return;
    }
    adopt(res.state);
    setVersions(null);
    router.refresh();
  });

  function fail(f: Parameters<typeof projectFailureText>[0]) {
    if (f.kind === "unauthenticated") {
      router.replace("/login");
      return;
    }
    setError(projectFailureText(f));
  }

  async function reload() {
    const res = await api.readme(projectId);
    if (res.ok) setLoad({ kind: "ready", state: res.state });
  }

  async function generate() {
    if (busy) return;
    setBusy(true);
    setError(null);
    const res = await api.generateReadme(csrf, projectId, template);
    setBusy(false);
    if (!res.ok) return fail(res);
    setBlank(false);
    await reload();
  }

  async function save() {
    if (busy || !state) return;
    setBusy(true);
    setError(null);
    const res = await api.saveReadme(csrf, projectId, draft, state.readme?.version ?? 0);
    setBusy(false);
    if (!res.ok) return fail(res);
    setBlank(false);
    setVersions(null);
    const next = await api.readme(projectId);
    if (next.ok) setLoad({ kind: "ready", state: next.state });
  }

  async function rewriteSection() {
    if (busy || !rewrite) return;
    setBusy(true);
    setError(null);
    const res = await api.rewriteSection(csrf, projectId, rewrite.heading, rewrite.instruction);
    setBusy(false);
    if (!res.ok) return fail(res);
    setRewrite(null);
    await reload();
  }

  async function showHistory() {
    const res = await api.readmeVersions(projectId);
    if (res.ok) setVersions(res.versions);
    else fail(res);
  }

  async function restore(version: number) {
    if (busy) return;
    setBusy(true);
    setError(null);
    const res = await api.restoreReadme(csrf, projectId, version);
    setBusy(false);
    if (!res.ok) return fail(res);
    setVersions(null);
    const next = await api.readme(projectId);
    if (next.ok) adopt(next.state);
  }

  function download() {
    const url = URL.createObjectURL(new Blob([draft], { type: "text/markdown" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "README.md";
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }

  if (load.kind === "loading") {
    return (
      <div role="status" aria-label="Loading the README" className="flex flex-col gap-4">
        <Skeleton className="h-12 w-full" />
        <div className="grid gap-4 lg:grid-cols-2">
          <Skeleton className="h-[28rem] rounded-card" />
          <Skeleton className="h-[28rem] rounded-card" />
        </div>
      </div>
    );
  }
  if (load.kind === "error" || state === null) {
    return <Message tone="error">We could not load the README. Try reloading the page.</Message>;
  }

  const job = state.job;
  const jobNote =
    job && job.status === "failed" ? (
      <Message tone="info">{readmeFailureText(job.error_code)}</Message>
    ) : null;

  const picker = (
    <RadioCardGroup
      aria-label="README layout"
      value={template}
      onValueChange={(v) => setTemplate(v as ReadmeTemplate)}
      className="sm:grid-cols-3"
    >
      {(Object.keys(TEMPLATE_LABELS) as ReadmeTemplate[]).map((t) => (
        <RadioCard key={t} value={t} title={TEMPLATE_LABELS[t]} description={TEMPLATE_HINTS[t]} />
      ))}
    </RadioCardGroup>
  );

  const progress =
    working && job ? (
      <Card aria-live="polite">
        <CardContent className="flex flex-col gap-3 pt-6">
          <Progress value={job.progress} label="README progress" />
          <span className="text-muted-foreground text-sm">{readmeStepLabel(job.step)}…</span>
        </CardContent>
      </Card>
    ) : null;

  if (!state.readme && !blank) {
    return (
      <div className="flex flex-col gap-5">
        {progress}
        {!working ? (
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Write your README</CardTitle>
              <CardDescription>
                Written from the analysis, then yours to edit. Every edit is a version you can go
                back to.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-5">
              {picker}
              <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                <Button variant="accent" size="lg" busy={busy} onClick={() => void generate()}>
                  <Sparkles aria-hidden /> Write my README
                </Button>
                <Button
                  variant="secondary"
                  size="lg"
                  disabled={busy}
                  onClick={() => {
                    setBlank(true);
                    setDraft(`# ${repoName.split("/").pop() ?? repoName}\n`);
                  }}
                >
                  Start from a blank page
                </Button>
                <span className="text-muted-foreground text-xs">
                  Up to {GENERATE_COST} credits; you pay what it costs and the rest is returned.
                </span>
              </div>
            </CardContent>
          </Card>
        ) : null}
        {jobNote}
        {error ? <Message tone="error">{error}</Message> : null}
      </div>
    );
  }

  const saved = state.readme?.content ?? "";
  const dirty = draft !== saved;
  const headings = sectionHeadings(saved);
  const score = state.score;

  return (
    <div className="flex flex-col gap-6">
      {progress}
      {jobNote}

      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="primary"
          busy={busy}
          disabled={working || !dirty}
          onClick={() => void save()}
        >
          <Save aria-hidden />
          {dirty ? "Save changes" : "Saved"}
        </Button>
        <CopyButton text={draft} />
        <Button variant="secondary" onClick={download}>
          <Download aria-hidden /> Download README.md
        </Button>
        <span className="ml-auto flex items-center gap-2">
          {dirty ? (
            <Badge tone="warn" dot>
              Unsaved changes
            </Badge>
          ) : null}
          <span className="text-muted-foreground text-xs">
            {state.readme
              ? `Version ${state.readme.version} · ${SOURCE_LABELS[state.readme.source]}`
              : "Not saved yet"}
          </span>
        </span>
      </div>

      <div
        role="group"
        aria-label="View"
        className="border-border bg-surface-2 inline-flex w-fit gap-1 rounded-control border p-1 lg:hidden"
      >
        {(["edit", "preview"] as const).map((p) => (
          <button
            key={p}
            type="button"
            aria-pressed={pane === p}
            onClick={() => setPane(p)}
            className={cn(
              "rounded-md px-3 py-1 text-[13px] font-semibold capitalize transition-colors",
              pane === p ? "bg-card shadow-soft" : "text-muted-foreground",
            )}
          >
            {p}
          </button>
        ))}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className={cn("flex flex-col gap-2", pane === "preview" && "max-lg:hidden")}>
          <label htmlFor="readme-editor" className="text-[13px] font-semibold">
            Markdown
          </label>
          <textarea
            id="readme-editor"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            disabled={working}
            spellCheck={false}
            maxLength={100_000}
            className="border-border bg-card focus-visible:border-accent focus-visible:ring-accent/15 scroll-thin min-h-[30rem] w-full resize-y rounded-card border p-4 font-mono text-xs leading-relaxed focus-visible:ring-4 focus-visible:outline-none disabled:opacity-60 lg:h-[34rem]"
          />
        </div>
        <div className={cn("flex flex-col gap-2", pane === "edit" && "max-lg:hidden")}>
          <span className="text-[13px] font-semibold">Preview</span>
          <div
            aria-label="README preview"
            className="border-border bg-card scroll-thin max-h-[34rem] min-h-[30rem] overflow-auto rounded-card border p-6 lg:h-[34rem]"
          >
            <MarkdownView source={draft} />
          </div>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        {score ? (
          <Card aria-label="README health">
            <CardHeader className="pb-3">
              <div className="flex items-baseline justify-between gap-3">
                <CardTitle className="text-base">Health</CardTitle>
                <span className="font-mono text-sm">
                  <b className="text-accent text-lg">{score.score}</b> / 100
                </span>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <Progress value={score.score} label="README health score" />
              <ul className="flex flex-col gap-2">
                {score.items.map((i) => (
                  <li key={i.key} className="flex items-start gap-2 text-sm">
                    {i.ok ? (
                      <CheckCircle2 className="text-ok mt-0.5 size-4 shrink-0" aria-hidden />
                    ) : (
                      <XCircle className="text-faint mt-0.5 size-4 shrink-0" aria-hidden />
                    )}
                    <span>
                      <span className={i.ok ? "" : "font-semibold"}>{i.label}</span>
                      <span className="sr-only">{i.ok ? ": present" : ": missing"}</span>
                      {i.ok ? null : <span className="text-muted-foreground"> — {i.hint}</span>}
                    </span>
                  </li>
                ))}
              </ul>
              <p className="text-faint text-xs">
                This checks the README&apos;s structure, not how good it is.
              </p>
            </CardContent>
          </Card>
        ) : null}

        {headings.length > 0 ? (
          <Card aria-label="Rewrite a section">
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Rewrite a section</CardTitle>
              <CardDescription>
                Fix one weak part without redoing the whole document.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              <ul className="divide-border flex flex-col divide-y">
                {headings.map((h) => (
                  <li key={h} className="flex flex-col gap-3 py-2.5 first:pt-0">
                    <div className="flex items-center justify-between gap-3 text-sm">
                      <span className="min-w-0 truncate">{h}</span>
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={busy || working || dirty}
                        onClick={() => setRewrite({ heading: h, instruction: "" })}
                        aria-label={`Rewrite the ${h} section`}
                      >
                        <Wand2 aria-hidden /> Rewrite
                      </Button>
                    </div>
                    {rewrite?.heading === h ? (
                      <div className="animate-fade-in border-border bg-surface-2 flex flex-col gap-3 rounded-control border p-3">
                        <Field
                          label={
                            <span className="text-xs font-normal">
                              Anything it should do? For example &quot;shorter&quot; or &quot;add
                              Docker steps&quot;. Optional.
                            </span>
                          }
                        >
                          {(field) => (
                            <Input
                              {...field}
                              value={rewrite.instruction}
                              maxLength={300}
                              onChange={(e) =>
                                setRewrite({ heading: h, instruction: e.target.value })
                              }
                            />
                          )}
                        </Field>
                        <div className="flex flex-wrap gap-2">
                          <Button busy={busy} onClick={() => void rewriteSection()}>
                            Rewrite ({SECTION_COST} credits at most)
                          </Button>
                          <Button variant="secondary" onClick={() => setRewrite(null)}>
                            Cancel
                          </Button>
                        </div>
                      </div>
                    ) : null}
                  </li>
                ))}
              </ul>
              {dirty ? (
                <p className="text-muted-foreground text-xs">
                  Save your edits before rewriting a section.
                </p>
              ) : null}
            </CardContent>
          </Card>
        ) : null}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card aria-label="Write it again">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Write it again from scratch</CardTitle>
            <CardDescription>A fresh start. The old README stays in the history.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {picker}
            <div>
              <Button
                variant="secondary"
                busy={busy}
                disabled={working}
                onClick={() => void generate()}
              >
                <FilePlus2 aria-hidden /> Write a new README ({GENERATE_COST} credits at most)
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card aria-label="History">
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between gap-3">
              <CardTitle className="text-base">History</CardTitle>
              {versions === null ? (
                <Button variant="secondary" size="sm" onClick={() => void showHistory()}>
                  <History aria-hidden /> Show versions
                </Button>
              ) : null}
            </div>
            <CardDescription>No change is ever lost, so experimenting is safe.</CardDescription>
          </CardHeader>
          {versions ? (
            <CardContent>
              <ul className="divide-border divide-y">
                {versions.map((v) => (
                  <li
                    key={v.version}
                    className="flex items-center justify-between gap-3 py-2.5 text-sm"
                  >
                    <span className="min-w-0">
                      <span className="font-medium">
                        Version {v.version} · {SOURCE_LABELS[v.source]}
                      </span>
                      <time dateTime={v.created_at} className="text-muted-foreground block text-xs">
                        {new Date(v.created_at).toLocaleString("en-US", {
                          dateStyle: "medium",
                          timeStyle: "short",
                        })}
                      </time>
                    </span>
                    {v.version === state.readme?.version ? (
                      <Badge tone="accent">Current</Badge>
                    ) : (
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={busy || working}
                        onClick={() => void restore(v.version)}
                        aria-label={`Restore version ${v.version}`}
                      >
                        <RotateCcw aria-hidden /> Restore
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            </CardContent>
          ) : null}
        </Card>
      </div>

      {error ? <Message tone="error">{error}</Message> : null}
    </div>
  );
}
