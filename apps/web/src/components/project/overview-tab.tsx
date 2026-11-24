"use client";

import { RefreshCw, Sparkles } from "lucide-react";
import { AnalysisPanel } from "@/components/analysis-panel";
import { useProject } from "@/components/project/project-provider";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { Message } from "@/components/ui/message";
import { Progress } from "@/components/ui/progress";
import { formatSize, jobFailureText, stepLabel } from "@/lib/project-text";

/** What Readmely learned about the repository, and the live progress while it learns. */
export function OverviewTab() {
  const { project, working, busy, analyse, api, analysisKey, bumpPack, error } = useProject();

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">About this repository</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-1.5 text-sm">
          <p>{project.description || "No description."}</p>
          <p className="text-muted-foreground text-xs">
            Default branch {project.default_branch || "unknown"} · {formatSize(project.size_kb)}
          </p>
        </CardContent>
      </Card>

      {working ? (
        <Card aria-live="polite">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Reading the repository</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <Progress value={project.job?.progress ?? 0} label="Analysis progress" />
            <p className="text-muted-foreground text-sm">
              {stepLabel(project.job?.step ?? null)}… This usually takes about a minute.
            </p>
          </CardContent>
        </Card>
      ) : null}

      {project.status === "failed" ? (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">The analysis did not finish</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Message tone="error">
              {jobFailureText(project.job?.error_code ?? null)} Nothing was charged.
            </Message>
            <div>
              <Button variant="accent" busy={busy} onClick={() => void analyse()}>
                Try again
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {project.status === "draft" ? (
        <EmptyState
          icon={<Sparkles aria-hidden />}
          title="This project has not been analysed yet"
          action={
            <Button variant="accent" size="lg" busy={busy} onClick={() => void analyse()}>
              Analyse
            </Button>
          }
        >
          Readmely reads the repository, then shows what it found here. Analysis holds 10 credits
          and is usually charged about 1.
        </EmptyState>
      ) : null}

      {project.status === "ready" ? (
        <>
          <AnalysisPanel
            key={analysisKey}
            api={api}
            projectId={project.id}
            onSummaryReady={bumpPack}
          />
          <div>
            <Button
              variant="secondary"
              busy={busy}
              disabled={working}
              onClick={() => void analyse()}
            >
              <RefreshCw aria-hidden /> Analyse again
            </Button>
          </div>
        </>
      ) : null}

      {error ? <Message tone="error">{error}</Message> : null}
    </div>
  );
}
