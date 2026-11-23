"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { createProjectsApi, type ProjectsApi } from "@/lib/projects-api";
import { isWorking, type Project } from "@/lib/project-schema";
import { statusBadge, stepLabel } from "@/lib/project-text";

export const POLL_MS = 2000;

/** Polls `load` while `active`, so a running analysis moves without a reload. */
export function usePolling(active: boolean, load: () => Promise<void>) {
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => void load(), POLL_MS);
    return () => clearInterval(timer);
    // `load` is re-created every render; the interval only needs to restart when `active` flips.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active]);
}

export function ProjectsView({ initial, api }: { initial: Project[]; api?: ProjectsApi }) {
  const router = useRouter();
  const [client] = useState(() => api ?? createProjectsApi());
  const [projects, setProjects] = useState<Project[]>(initial);

  usePolling(projects.some(isWorking), async () => {
    const res = await client.list();
    if (res.ok) setProjects(res.projects);
    else if (res.kind === "unauthenticated") router.replace("/login");
  });

  if (projects.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>No projects yet</CardTitle>
          <p className="text-muted-foreground text-sm">
            Paste the address of a public GitHub repository and Readmely will read it, then write a
            README, a context pack and an architecture diagram for it.
          </p>
        </CardHeader>
        <CardContent>
          <Link
            href="/projects/new"
            className="bg-foreground text-background inline-flex h-10 items-center rounded-md px-4 text-sm font-medium hover:opacity-90"
          >
            Add your first project
          </Link>
        </CardContent>
      </Card>
    );
  }

  return (
    <ul className="grid gap-4 sm:grid-cols-2" aria-label="Projects">
      {projects.map((p) => {
        const badge = statusBadge(p);
        return (
          <li key={p.id}>
            <Link
              href={`/projects/${p.id}`}
              className="border-border bg-card focus-visible:outline-foreground flex h-full flex-col gap-3 rounded-lg border p-4 hover:bg-border/20 focus-visible:outline-2"
            >
              <div className="flex items-start justify-between gap-3">
                <span className="font-medium break-all">{p.name}</span>
                <Badge tone={badge.tone}>{badge.label}</Badge>
              </div>
              <p className="text-muted-foreground line-clamp-2 text-sm">
                {p.description || "No description."}
              </p>
              {isWorking(p) ? (
                <div className="flex flex-col gap-1">
                  <Progress value={p.job?.progress ?? 0} label={`Analysis of ${p.name}`} />
                  <span className="text-muted-foreground text-xs">
                    {stepLabel(p.job?.step ?? null)}
                  </span>
                </div>
              ) : null}
            </Link>
          </li>
        );
      })}
    </ul>
  );
}
