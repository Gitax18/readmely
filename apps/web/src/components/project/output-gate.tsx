"use client";

import Link from "next/link";
import * as React from "react";
import { useProject } from "@/components/project/project-provider";
import { Message } from "@/components/ui/message";
import { Button, buttonVariants } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { Progress } from "@/components/ui/progress";
import { Sparkles } from "lucide-react";
import { stepLabel } from "@/lib/project-text";

/**
 * The README, context pack and diagram all need an analysed project. Until there is one this says
 * why and offers the way forward; once it is ready, the tab's own content is shown.
 */
export function OutputGate({ what, children }: { what: string; children: React.ReactNode }) {
  const { project, working, busy, analyse, error } = useProject();
  if (project.status === "ready") return <>{children}</>;
  if (working) {
    return (
      <div
        className="border-border bg-card shadow-soft flex flex-col gap-3 rounded-card border p-6"
        aria-live="polite"
      >
        <h2 className="text-lg font-semibold tracking-tight">Reading the repository…</h2>
        <Progress value={project.job?.progress ?? 0} label="Analysis progress" />
        <p className="text-muted-foreground text-sm">
          {stepLabel(project.job?.step ?? null)}. The {what} is available as soon as this finishes,
          usually in about a minute.
        </p>
      </div>
    );
  }
  return (
    <EmptyState
      icon={<Sparkles aria-hidden />}
      title="Analyse the project first"
      action={
        <div className="flex flex-wrap items-center justify-center gap-3">
          <Button variant="accent" size="lg" busy={busy} onClick={() => void analyse()}>
            {project.status === "failed" ? "Try again" : "Analyse"}
          </Button>
          <Link
            href={`/projects/${project.id}`}
            className={buttonVariants({ variant: "ghost", size: "lg" })}
          >
            Back to overview
          </Link>
        </div>
      }
    >
      Readmely writes the {what} from what it finds in the code, so it needs to read the repository
      first.
      {error ? (
        <Message tone="error" className="mt-4 text-left">
          {error}
        </Message>
      ) : null}
    </EmptyState>
  );
}
