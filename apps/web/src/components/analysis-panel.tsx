"use client";

import {
  Boxes,
  CheckCircle2,
  FileCode2,
  MinusCircle,
  ShieldCheck,
  ShieldAlert,
  Terminal,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { usePolling } from "@/components/projects-view";
import { Badge } from "@/components/ui/badge";
import { Bar } from "@/components/ui/bar";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Code } from "@/components/ui/code";
import { Message } from "@/components/ui/message";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import type { ProjectAnalysis } from "@/lib/analysis-schema";
import type { ProjectsApi } from "@/lib/projects-api";
import {
  formatCount,
  formatSize,
  secretRuleLabel,
  skipReasonLabel,
  summaryFailureText,
  summaryStepLabel,
} from "@/lib/project-text";

type State = { kind: "loading" } | { kind: "error" } | { kind: "ready"; analysis: ProjectAnalysis };

function summaryWorking(a: ProjectAnalysis): boolean {
  return (
    a.summary === null &&
    a.summary_job !== null &&
    ["queued", "running"].includes(a.summary_job.status)
  );
}

/** What the last analysis found. Mount it with a key that changes per analysis to reload it. */
export function AnalysisPanel({
  api,
  projectId,
  onSummaryReady,
}: {
  api: ProjectsApi;
  projectId: string;
  onSummaryReady?: () => void;
}) {
  const router = useRouter();
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    let live = true;
    void api.analysis(projectId).then((res) => {
      if (!live) return;
      setState(res.ok ? { kind: "ready", analysis: res.analysis } : { kind: "error" });
    });
    return () => {
      live = false;
    };
  }, [api, projectId]);

  // The summary arrives after the rest: poll until it settles, then refresh so the credit balance
  // in the top bar shows what it cost.
  const working = state.kind === "ready" && summaryWorking(state.analysis);
  usePolling(working, async () => {
    const res = await api.analysis(projectId);
    if (!res.ok) return;
    setState({ kind: "ready", analysis: res.analysis });
    if (!summaryWorking(res.analysis)) {
      router.refresh();
      onSummaryReady?.();
    }
  });

  if (state.kind === "loading") {
    return (
      <div role="status" aria-label="Loading what we found" className="flex flex-col gap-4">
        <Skeleton className="h-44 w-full rounded-card" />
        <div className="grid gap-4 sm:grid-cols-2">
          <Skeleton className="h-32 rounded-card" />
          <Skeleton className="h-32 rounded-card" />
        </div>
      </div>
    );
  }
  if (state.kind === "error") {
    return <Message tone="error">We could not load the results. Try reloading the page.</Message>;
  }
  return <Findings analysis={state.analysis} />;
}

function Panel({
  title,
  icon,
  children,
  className,
}: {
  title: string;
  icon?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm [&>svg]:size-4 [&>svg]:text-accent">
          {icon}
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">{children}</CardContent>
    </Card>
  );
}

function SummarySection({ analysis }: { analysis: ProjectAnalysis }) {
  const { summary, summary_job: job } = analysis;
  if (summary) {
    return (
      <Card className="border-accent/25">
        <CardHeader className="pb-3">
          <div className="text-accent text-[11px] font-bold tracking-[0.12em] uppercase">
            What this project does
          </div>
          <CardTitle className="text-xl leading-snug">{summary.headline}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <p className="text-muted-foreground max-w-[75ch] text-sm leading-relaxed">
            {summary.overview}
          </p>
          {summary.how_it_works.length > 0 ? (
            <div className="flex flex-col gap-2">
              <h3 className="text-sm font-semibold">How it works</h3>
              <ol className="flex flex-col gap-2">
                {summary.how_it_works.map((step, i) => (
                  <li key={step} className="flex gap-3 text-sm">
                    <span className="bg-accent-soft text-accent grid size-5 shrink-0 place-items-center rounded-full font-mono text-[10px] font-semibold">
                      {i + 1}
                    </span>
                    <span className="text-muted-foreground">{step}</span>
                  </li>
                ))}
              </ol>
            </div>
          ) : null}
          {summary.components.length > 0 ? (
            <div className="flex flex-col gap-2">
              <h3 className="text-sm font-semibold">Key files</h3>
              <ul className="flex flex-col gap-2">
                {summary.components.map((c) => (
                  <li
                    key={c.path}
                    className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm"
                  >
                    <Code>{c.path}</Code>
                    <span className="text-muted-foreground">{c.role}</span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          <p className="text-faint text-xs">
            Written by an AI model from the analysis. It can be wrong; check it against the code.
          </p>
        </CardContent>
      </Card>
    );
  }
  if (job && ["queued", "running"].includes(job.status)) {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">What this project does</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2" aria-live="polite">
          <Progress value={job.progress} label="Summary progress" />
          <span className="text-muted-foreground text-sm">{summaryStepLabel(job.step)}…</span>
        </CardContent>
      </Card>
    );
  }
  if (job && job.status === "failed") {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">What this project does</CardTitle>
        </CardHeader>
        <CardContent>
          <p role="status" className="text-muted-foreground text-sm">
            {summaryFailureText(job.error_code)}
          </p>
        </CardContent>
      </Card>
    );
  }
  return null;
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-border bg-card rounded-xl border px-4 py-3">
      <dt className="text-faint text-[11px] font-semibold tracking-wide uppercase">{label}</dt>
      <dd className="mt-0.5 font-mono text-sm font-medium break-all">{value}</dd>
    </div>
  );
}

function Findings({ analysis }: { analysis: ProjectAnalysis }) {
  const f = analysis.facts;
  const checks: [string, boolean][] = [
    ["README", f.has_readme],
    ["Tests", f.has_tests],
    ["CI", f.has_ci],
    ["Docker", f.has_docker],
  ];
  return (
    <div className="flex flex-col gap-4">
      <SummarySection analysis={analysis} />

      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        <Stat label="Commit" value={analysis.commit_sha.slice(0, 7)} />
        <Stat label="Files" value={formatCount(f.totals.files)} />
        <Stat label="Lines" value={formatCount(f.totals.lines)} />
        <Stat label="Size" value={formatSize(Math.ceil(f.totals.bytes / 1024))} />
        <Stat label="Licence" value={f.license || "None found"} />
      </dl>

      <div className="grid gap-4 md:grid-cols-2">
        {f.languages.length > 0 ? (
          <Panel title="Languages" icon={<FileCode2 aria-hidden />}>
            <ul className="flex flex-col gap-2.5">
              {f.languages.map((l) => (
                <li key={l.name} className="flex items-center gap-3 text-sm">
                  <span className="w-24 shrink-0 truncate">{l.name}</span>
                  <div role="img" aria-label={`${l.name} ${l.percent} percent`} className="flex-1">
                    <Bar percent={Math.max(2, l.percent)} />
                  </div>
                  <span className="text-muted-foreground w-12 text-right font-mono text-xs">
                    {l.percent}%
                  </span>
                </li>
              ))}
            </ul>
          </Panel>
        ) : null}

        <Panel title="Built with" icon={<Boxes aria-hidden />}>
          <Chips items={f.frameworks} empty="No frameworks recognised." />
          {f.package_managers.length > 0 ? (
            <p className="text-muted-foreground text-xs">
              Package managers: {f.package_managers.join(", ")}
            </p>
          ) : null}
        </Panel>

        <Panel title="Project health" icon={<CheckCircle2 aria-hidden />}>
          <ul className="grid grid-cols-2 gap-2">
            {checks.map(([label, ok]) => (
              <li key={label}>
                <Badge tone={ok ? "ok" : "neutral"} className="w-full justify-start">
                  {ok ? (
                    <CheckCircle2 className="size-3.5" aria-hidden />
                  ) : (
                    <MinusCircle className="size-3.5" aria-hidden />
                  )}
                  {label}: {ok ? "found" : "not found"}
                </Badge>
              </li>
            ))}
          </ul>
        </Panel>

        {f.entry_points.length > 0 ? (
          <Panel title="Where it starts">
            <CodeList items={f.entry_points} />
          </Panel>
        ) : null}

        {f.commands.length > 0 ? (
          <Panel title="Commands" icon={<Terminal aria-hidden />}>
            <ul className="flex flex-col gap-1.5">
              {f.commands.map((c) => (
                <li key={`${c.source}:${c.name}`}>
                  <Code>{c.name}</Code>
                </li>
              ))}
            </ul>
          </Panel>
        ) : null}

        {f.services.length > 0 ? (
          <Panel title="Services">
            <Chips items={f.services} empty="" />
          </Panel>
        ) : null}

        {f.env_vars.length > 0 ? (
          <Panel title="Settings it reads">
            <CodeList items={f.env_vars} inline />
          </Panel>
        ) : null}

        {analysis.hubs.length > 0 ? (
          <Panel title="Most imported">
            <ul className="flex flex-col gap-2">
              {analysis.hubs.map((h) => (
                <li
                  key={`${h.kind}:${h.key}`}
                  className="flex items-baseline justify-between gap-3 text-sm"
                >
                  <Code>{h.key}</Code>
                  <span className="text-muted-foreground shrink-0 text-xs">
                    by {formatCount(h.imported_by)}
                  </span>
                </li>
              ))}
            </ul>
          </Panel>
        ) : null}
      </div>

      <Panel
        title="Secrets"
        icon={f.secrets.count === 0 ? <ShieldCheck aria-hidden /> : <ShieldAlert aria-hidden />}
      >
        {f.secrets.count === 0 ? (
          <p className="text-sm">None found.</p>
        ) : (
          <>
            <Message tone="error">
              {formatCount(f.secrets.count)} secret-like value{f.secrets.count === 1 ? "" : "s"}{" "}
              found and removed before anything was stored. If they are real, rotate them: they are
              in your repository&apos;s history.
            </Message>
            <ul className="flex flex-col gap-1.5">
              {f.secrets.locations.slice(0, 10).map((s) => (
                <li key={`${s.path}:${s.line}:${s.rule}`} className="text-xs">
                  {secretRuleLabel(s.rule)} · <Code>{s.path}</Code>, line {s.line}
                </li>
              ))}
            </ul>
          </>
        )}
      </Panel>

      {f.skipped.count > 0 ? (
        <p className="text-muted-foreground text-xs">
          Not read: {formatCount(f.skipped.count)} files (
          {Object.entries(f.skipped.by_reason)
            .map(([r, n]) => `${formatCount(n)} ${skipReasonLabel(r)}`)
            .join(", ")}
          ).
        </p>
      ) : null}
    </div>
  );
}

function Chips({ items, empty }: { items: string[]; empty: string }) {
  if (items.length === 0) return empty ? <p className="text-sm">{empty}</p> : null;
  return (
    <ul className="flex flex-wrap gap-2">
      {items.map((i) => (
        <li key={i}>
          <Badge tone="neutral">{i}</Badge>
        </li>
      ))}
    </ul>
  );
}

function CodeList({ items, inline = false }: { items: string[]; inline?: boolean }) {
  return (
    <ul className={inline ? "flex flex-wrap gap-2" : "flex flex-col gap-1.5"}>
      {items.map((i) => (
        <li key={i}>
          <Code>{i}</Code>
        </li>
      ))}
    </ul>
  );
}
