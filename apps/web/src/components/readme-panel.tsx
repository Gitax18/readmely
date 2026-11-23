"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { usePolling } from "@/components/projects-view";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
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
  const [copied, setCopied] = useState(false);
  const [versions, setVersions] = useState<ReadmeVersions["versions"] | null>(null);
  const [rewrite, setRewrite] = useState<{ heading: string; instruction: string } | null>(null);

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

  async function copy() {
    try {
      await navigator.clipboard.writeText(draft);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
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
    return <p className="text-muted-foreground text-sm">Loading the README…</p>;
  }
  if (load.kind === "error" || state === null) {
    return (
      <p className="text-muted-foreground text-sm">
        We could not load the README. Try reloading the page.
      </p>
    );
  }

  const job = state.job;
  const jobNote =
    job && job.status === "failed" ? (
      <p role="status" className="text-muted-foreground text-sm">
        {readmeFailureText(job.error_code)}
      </p>
    ) : null;
  const picker = (
    <fieldset className="flex flex-col gap-2">
      <legend className="sr-only">README layout</legend>
      <div className="flex flex-wrap gap-2">
        {(Object.keys(TEMPLATE_LABELS) as ReadmeTemplate[]).map((t) => (
          <label
            key={t}
            className={`border-border cursor-pointer rounded-md border px-3 py-1.5 text-sm ${
              t === template ? "bg-foreground text-background" : ""
            }`}
          >
            <input
              type="radio"
              name="readme-template"
              value={t}
              checked={t === template}
              onChange={() => setTemplate(t)}
              className="sr-only"
            />
            {TEMPLATE_LABELS[t]}
          </label>
        ))}
      </div>
      <p className="text-muted-foreground text-xs">{TEMPLATE_HINTS[template]}</p>
    </fieldset>
  );

  const working_ =
    working && job ? (
      <div className="flex flex-col gap-2" aria-live="polite">
        <Progress value={job.progress} label="README progress" />
        <span className="text-sm">{readmeStepLabel(job.step)}…</span>
      </div>
    ) : null;

  if (!state.readme && !blank) {
    return (
      <div className="flex flex-col gap-4">
        {working_}
        {!working ? (
          <>
            {picker}
            <div className="flex flex-wrap items-center gap-3">
              <Button onClick={() => void generate()} disabled={busy}>
                Write my README
              </Button>
              <Button
                variant="secondary"
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
          </>
        ) : null}
        {jobNote}
        {error ? (
          <p role="alert" className="text-bad text-sm">
            {error}
          </p>
        ) : null}
      </div>
    );
  }

  const saved = state.readme?.content ?? "";
  const dirty = draft !== saved;
  const headings = sectionHeadings(saved);
  const score = state.score;

  return (
    <div className="flex flex-col gap-5">
      {working_}
      {jobNote}

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="flex flex-col gap-2">
          <label htmlFor="readme-editor" className="text-sm font-medium">
            Markdown
          </label>
          <textarea
            id="readme-editor"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            disabled={working}
            spellCheck={false}
            maxLength={100_000}
            className="border-border bg-card min-h-[28rem] w-full rounded-md border p-3 font-mono text-xs focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-foreground disabled:opacity-60"
          />
        </div>
        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium">Preview</span>
          <div
            aria-label="README preview"
            className="border-border max-h-[32rem] min-h-[28rem] overflow-auto rounded-md border p-4 text-sm [&_a]:underline [&_code]:text-xs [&_h1]:mb-3 [&_h1]:text-xl [&_h1]:font-semibold [&_h2]:mt-5 [&_h2]:mb-2 [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:mt-4 [&_h3]:mb-1 [&_h3]:font-semibold [&_li]:ml-5 [&_li]:list-disc [&_p]:mb-3 [&_pre]:border-border [&_pre]:mb-3 [&_pre]:overflow-auto [&_pre]:rounded [&_pre]:border [&_pre]:p-2 [&_table]:mb-3 [&_td]:border [&_td]:px-2 [&_th]:border [&_th]:px-2"
          >
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                a: ({ href, children }) => (
                  <a href={href} target="_blank" rel="noreferrer noopener nofollow">
                    {children}
                  </a>
                ),
                // Never let a README make the browser fetch an address of its choosing.
                img: ({ alt }) => <span>[{alt || "image"}]</span>,
              }}
            >
              {draft}
            </ReactMarkdown>
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={() => void save()} disabled={busy || working || !dirty}>
          {dirty ? "Save changes" : "Saved"}
        </Button>
        <Button variant="secondary" onClick={() => void copy()}>
          {copied ? "Copied" : "Copy"}
        </Button>
        <Button variant="secondary" onClick={download}>
          Download README.md
        </Button>
        {state.readme ? (
          <span className="text-muted-foreground text-xs">
            Version {state.readme.version} · {SOURCE_LABELS[state.readme.source]}
          </span>
        ) : (
          <span className="text-muted-foreground text-xs">Not saved yet</span>
        )}
      </div>

      {score ? (
        <section className="flex flex-col gap-2" aria-label="README health">
          <h3 className="text-sm font-medium">Health: {score.score} / 100</h3>
          <Progress value={score.score} label="README health score" />
          <ul className="flex flex-col gap-1">
            {score.items.map((i) => (
              <li key={i.key} className="text-sm">
                <span aria-hidden>{i.ok ? "✓" : "✗"}</span>{" "}
                <span className={i.ok ? "" : "font-medium"}>{i.label}</span>
                <span className="sr-only">{i.ok ? ": present" : ": missing"}</span>
                {i.ok ? null : <span className="text-muted-foreground"> — {i.hint}</span>}
              </li>
            ))}
          </ul>
          <p className="text-muted-foreground text-xs">
            This checks the README&apos;s structure, not how good it is.
          </p>
        </section>
      ) : null}

      {headings.length > 0 ? (
        <section className="flex flex-col gap-2" aria-label="Rewrite a section">
          <h3 className="text-sm font-medium">Rewrite a section</h3>
          <ul className="flex flex-col gap-1.5">
            {headings.map((h) => (
              <li key={h} className="flex flex-col gap-2">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span>{h}</span>
                  <Button
                    variant="secondary"
                    className="h-8 px-3"
                    disabled={busy || working || dirty}
                    onClick={() => setRewrite({ heading: h, instruction: "" })}
                    aria-label={`Rewrite the ${h} section`}
                  >
                    Rewrite
                  </Button>
                </div>
                {rewrite?.heading === h ? (
                  <div className="border-border flex flex-col gap-2 rounded-md border p-3">
                    <label htmlFor="rewrite-instruction" className="text-xs">
                      Anything it should do? For example &quot;shorter&quot; or &quot;add Docker
                      steps&quot;. Optional.
                    </label>
                    <Input
                      id="rewrite-instruction"
                      value={rewrite.instruction}
                      maxLength={300}
                      onChange={(e) => setRewrite({ heading: h, instruction: e.target.value })}
                    />
                    <div className="flex gap-2">
                      <Button onClick={() => void rewriteSection()} disabled={busy}>
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
        </section>
      ) : null}

      <section className="flex flex-col gap-2" aria-label="Write it again">
        <h3 className="text-sm font-medium">Write it again from scratch</h3>
        {picker}
        <div>
          <Button variant="secondary" onClick={() => void generate()} disabled={busy || working}>
            Write a new README ({GENERATE_COST} credits at most)
          </Button>
        </div>
      </section>

      <section className="flex flex-col gap-2" aria-label="History">
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-medium">History</h3>
          {versions === null ? (
            <Button variant="secondary" className="h-8 px-3" onClick={() => void showHistory()}>
              Show versions
            </Button>
          ) : null}
        </div>
        {versions ? (
          <ul className="divide-border divide-y">
            {versions.map((v) => (
              <li key={v.version} className="flex items-center justify-between gap-3 py-2 text-sm">
                <span>
                  Version {v.version} · {SOURCE_LABELS[v.source]}
                  <time dateTime={v.created_at} className="text-muted-foreground ml-2 text-xs">
                    {new Date(v.created_at).toLocaleString("en-US", {
                      dateStyle: "medium",
                      timeStyle: "short",
                    })}
                  </time>
                </span>
                {v.version === state.readme?.version ? (
                  <span className="text-muted-foreground text-xs">Current</span>
                ) : (
                  <Button
                    variant="secondary"
                    className="h-8 px-3"
                    disabled={busy || working}
                    onClick={() => void restore(v.version)}
                    aria-label={`Restore version ${v.version}`}
                  >
                    Restore
                  </Button>
                )}
              </li>
            ))}
          </ul>
        ) : null}
      </section>

      {error ? (
        <p role="alert" className="text-bad text-sm">
          {error}
        </p>
      ) : null}
    </div>
  );
}
