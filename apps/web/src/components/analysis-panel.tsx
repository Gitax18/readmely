"use client";

import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import type { ProjectAnalysis } from "@/lib/analysis-schema";
import type { ProjectsApi } from "@/lib/projects-api";
import { formatCount, formatSize, secretRuleLabel, skipReasonLabel } from "@/lib/project-text";

type State = { kind: "loading" } | { kind: "error" } | { kind: "ready"; analysis: ProjectAnalysis };

/** What the last analysis found. Mount it with a key that changes per analysis to reload it. */
export function AnalysisPanel({ api, projectId }: { api: ProjectsApi; projectId: string }) {
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

  if (state.kind === "loading") {
    return <p className="text-muted-foreground text-sm">Loading what we found…</p>;
  }
  if (state.kind === "error") {
    return (
      <p className="text-muted-foreground text-sm">
        We could not load the results. Try reloading the page.
      </p>
    );
  }
  return <Findings analysis={state.analysis} />;
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
      <p className="text-muted-foreground text-xs">
        Commit {analysis.commit_sha.slice(0, 7)} · {formatCount(f.totals.files)} files ·{" "}
        {formatCount(f.totals.lines)} lines · {formatSize(Math.ceil(f.totals.bytes / 1024))}
        {f.license ? ` · ${f.license} licence` : ""}
      </p>

      {f.languages.length > 0 ? (
        <Section title="Languages">
          <ul className="flex flex-col gap-1.5">
            {f.languages.map((l) => (
              <li key={l.name} className="flex items-center gap-3 text-sm">
                <span className="w-28 shrink-0">{l.name}</span>
                <div
                  className="bg-muted h-2 flex-1 overflow-hidden rounded-full"
                  role="img"
                  aria-label={`${l.name} ${l.percent} percent`}
                >
                  <div
                    className="bg-primary h-full"
                    style={{ width: `${Math.max(2, Math.min(100, l.percent))}%` }}
                  />
                </div>
                <span className="text-muted-foreground w-12 text-right text-xs">{l.percent}%</span>
              </li>
            ))}
          </ul>
        </Section>
      ) : null}

      <Section title="Built with">
        <Chips items={f.frameworks} empty="No frameworks recognised." />
        {f.package_managers.length > 0 ? (
          <p className="text-muted-foreground text-xs">
            Package managers: {f.package_managers.join(", ")}
          </p>
        ) : null}
      </Section>

      <Section title="Project health">
        <div className="flex flex-wrap gap-2">
          {checks.map(([label, ok]) => (
            <Badge key={label} tone={ok ? "ok" : "neutral"}>
              {label}: {ok ? "found" : "not found"}
            </Badge>
          ))}
        </div>
      </Section>

      {f.entry_points.length > 0 ? (
        <Section title="Where it starts">
          <CodeList items={f.entry_points} />
        </Section>
      ) : null}

      {f.commands.length > 0 ? (
        <Section title="Commands">
          <ul className="flex flex-col gap-1">
            {f.commands.map((c) => (
              <li key={`${c.source}:${c.name}`} className="text-sm">
                <code className="bg-muted rounded px-1.5 py-0.5 text-xs">{c.name}</code>
              </li>
            ))}
          </ul>
        </Section>
      ) : null}

      {f.services.length > 0 ? (
        <Section title="Services">
          <Chips items={f.services} empty="" />
        </Section>
      ) : null}

      {f.env_vars.length > 0 ? (
        <Section title="Settings it reads">
          <CodeList items={f.env_vars} inline />
        </Section>
      ) : null}

      {analysis.hubs.length > 0 ? (
        <Section title="Most imported">
          <ul className="flex flex-col gap-1">
            {analysis.hubs.map((h) => (
              <li key={`${h.kind}:${h.key}`} className="flex justify-between gap-3 text-sm">
                <code className="text-xs break-all">{h.key}</code>
                <span className="text-muted-foreground shrink-0 text-xs">
                  by {formatCount(h.imported_by)}
                </span>
              </li>
            ))}
          </ul>
        </Section>
      ) : null}

      <Section title="Secrets">
        {f.secrets.count === 0 ? (
          <p className="text-sm">None found.</p>
        ) : (
          <div className="flex flex-col gap-2">
            <p role="status" className="text-bad text-sm">
              {formatCount(f.secrets.count)} secret-like value
              {f.secrets.count === 1 ? "" : "s"} found and removed before anything was stored. If
              they are real, rotate them: they are in your repository&apos;s history.
            </p>
            <ul className="flex flex-col gap-1">
              {f.secrets.locations.slice(0, 10).map((s) => (
                <li key={`${s.path}:${s.line}:${s.rule}`} className="text-xs">
                  {secretRuleLabel(s.rule)} · <code className="break-all">{s.path}</code>, line{" "}
                  {s.line}
                </li>
              ))}
            </ul>
          </div>
        )}
      </Section>

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

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-sm font-medium">{title}</h3>
      {children}
    </section>
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
    <ul className={inline ? "flex flex-wrap gap-2" : "flex flex-col gap-1"}>
      {items.map((i) => (
        <li key={i}>
          <code className="bg-muted rounded px-1.5 py-0.5 text-xs break-all">{i}</code>
        </li>
      ))}
    </ul>
  );
}
