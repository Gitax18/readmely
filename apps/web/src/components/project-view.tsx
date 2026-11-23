"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { AnalysisPanel } from "@/components/analysis-panel";
import { ContextPackPanel } from "@/components/context-pack-panel";
import { usePolling } from "@/components/projects-view";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { createProjectsApi, type ProjectsApi } from "@/lib/projects-api";
import { isWorking, type Project } from "@/lib/project-schema";
import {
  formatSize,
  jobFailureText,
  projectFailureText,
  repoUrl,
  statusBadge,
  stepLabel,
} from "@/lib/project-text";

export function ProjectView({
  initial,
  csrf,
  api,
}: {
  initial: Project;
  csrf: string;
  api?: ProjectsApi;
}) {
  const router = useRouter();
  const [client] = useState(() => api ?? createProjectsApi());
  const [project, setProject] = useState<Project>(initial);
  const [error, setError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  // Bumped when the summary lands, so the pack is rebuilt with it.
  const [packVersion, setPackVersion] = useState(0);

  function fail(f: Parameters<typeof projectFailureText>[0]) {
    if (f.kind === "unauthenticated") {
      router.replace("/login");
      return;
    }
    setError(projectFailureText(f));
  }

  usePolling(isWorking(project), async () => {
    const res = await client.get(project.id);
    if (res.ok) setProject(res.project);
    else if (res.kind === "not_found") router.replace("/dashboard");
    else if (res.kind === "unauthenticated") router.replace("/login");
  });

  async function analyse() {
    if (busy) return;
    setBusy(true);
    setError(null);
    const res = await client.analyse(csrf, project.id);
    setBusy(false);
    if (res.ok) setProject(res.project);
    else fail(res);
  }

  async function remove() {
    if (busy) return;
    setBusy(true);
    setError(null);
    const res = await client.remove(csrf, project.id);
    if (res.ok || res.kind === "not_found") {
      router.replace("/dashboard");
      router.refresh();
      return;
    }
    setBusy(false);
    setConfirming(false);
    fail(res);
  }

  const badge = statusBadge(project);
  const working = isWorking(project);
  const link = repoUrl(project);
  const analyseLabel =
    project.status === "failed"
      ? "Try again"
      : project.status === "ready"
        ? "Analyse again"
        : "Analyse";

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold break-all">{project.name}</h1>
          {link ? (
            <a
              href={link}
              target="_blank"
              rel="noreferrer noopener"
              className="text-muted-foreground text-sm underline"
            >
              View on GitHub
            </a>
          ) : null}
        </div>
        <Badge tone={badge.tone}>{badge.label}</Badge>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>About this repository</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2 text-sm">
          <p>{project.description || "No description."}</p>
          <p className="text-muted-foreground text-xs">
            Default branch {project.default_branch || "unknown"} · {formatSize(project.size_kb)}
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Analysis</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {working ? (
            <div className="flex flex-col gap-2" aria-live="polite">
              <Progress value={project.job?.progress ?? 0} label="Analysis progress" />
              <span className="text-sm">{stepLabel(project.job?.step ?? null)}…</span>
            </div>
          ) : null}
          {project.status === "ready" ? (
            <AnalysisPanel
              key={project.job?.finished_at ?? project.updated_at}
              api={client}
              projectId={project.id}
              onSummaryReady={() => setPackVersion((v) => v + 1)}
            />
          ) : null}
          {project.status === "failed" ? (
            <p role="alert" className="text-bad text-sm">
              {jobFailureText(project.job?.error_code ?? null)} Nothing was charged.
            </p>
          ) : null}
          {project.status === "draft" ? (
            <p className="text-muted-foreground text-sm">This project has not been analysed yet.</p>
          ) : null}
          <div>
            <Button onClick={() => void analyse()} disabled={busy || working}>
              {analyseLabel}
            </Button>
          </div>
        </CardContent>
      </Card>

      {project.status === "ready" ? (
        <Card>
          <CardHeader>
            <CardTitle>Context pack</CardTitle>
            <p className="text-muted-foreground text-sm">
              Files that tell an AI coding tool what this repository is and how to work in it. Free:
              built from the analysis, no credits.
            </p>
          </CardHeader>
          <CardContent>
            <ContextPackPanel
              key={`${project.job?.finished_at ?? project.updated_at}:${packVersion}`}
              api={client}
              projectId={project.id}
              repoName={project.name}
            />
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Delete project</CardTitle>
          <p className="text-muted-foreground text-sm">
            Removes the project and everything generated for it. The repository itself is not
            touched.
          </p>
        </CardHeader>
        <CardContent>
          {confirming ? (
            <div
              role="group"
              aria-label="Confirm deleting this project"
              className="border-border flex flex-col gap-3 rounded-md border p-3"
            >
              <p className="text-sm">Delete {project.name}? This cannot be undone.</p>
              <div className="flex gap-2">
                <Button variant="danger" onClick={() => void remove()} disabled={busy}>
                  Yes, delete it
                </Button>
                <Button variant="secondary" onClick={() => setConfirming(false)} disabled={busy}>
                  Keep it
                </Button>
              </div>
            </div>
          ) : (
            <Button variant="danger" onClick={() => setConfirming(true)} disabled={busy}>
              Delete project
            </Button>
          )}
        </CardContent>
      </Card>

      {error ? (
        <p role="alert" className="text-bad text-sm">
          {error}
        </p>
      ) : null}
    </div>
  );
}
