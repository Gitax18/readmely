"use client";

import { FolderGit2, Plus } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { Progress } from "@/components/ui/progress";
import { createProjectsApi, type ProjectsApi } from "@/lib/projects-api";
import { timeAgo } from "@/lib/format-time";
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

function ProjectCard({ project: p }: { project: Project }) {
  const badge = statusBadge(p);
  const working = isWorking(p);
  return (
    <Link
      href={`/projects/${p.id}`}
      className="group focus-visible:outline-accent block h-full rounded-card focus-visible:outline-2 focus-visible:outline-offset-2"
    >
      <Card interactive className="flex h-full flex-col gap-3 p-5">
        <div className="flex items-start justify-between gap-3">
          <span className="flex min-w-0 items-center gap-2.5">
            <span className="bg-accent-soft text-accent grid size-9 shrink-0 place-items-center rounded-lg">
              <FolderGit2 className="size-[18px]" aria-hidden />
            </span>
            <span className="group-hover:text-accent font-semibold break-all transition-colors">
              {p.name}
            </span>
          </span>
          <Badge tone={badge.tone} dot={working ? "pulse" : true}>
            {badge.label}
          </Badge>
        </div>
        <p className="text-muted-foreground line-clamp-2 min-h-10 text-sm">
          {p.description || "No description."}
        </p>
        {working ? (
          <div className="flex flex-col gap-1.5">
            <Progress value={p.job?.progress ?? 0} label={`Analysis of ${p.name}`} />
            <span className="text-muted-foreground text-xs">{stepLabel(p.job?.step ?? null)}</span>
          </div>
        ) : (
          <p className="text-faint mt-auto text-xs" suppressHydrationWarning>
            Updated {timeAgo(p.updated_at)}
            {p.default_branch ? ` · ${p.default_branch}` : ""}
          </p>
        )}
      </Card>
    </Link>
  );
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
      <EmptyState
        icon={<FolderGit2 aria-hidden />}
        title="No projects yet"
        action={
          <Link href="/projects/new" className={buttonVariants({ variant: "accent", size: "lg" })}>
            <Plus aria-hidden /> Add your first project
          </Link>
        }
      >
        Paste the address of a public GitHub repository and Readmely will read it, then write a
        README, a context pack and an architecture diagram for it.
      </EmptyState>
    );
  }

  return (
    <ul className="stagger grid gap-4 sm:grid-cols-2" aria-label="Projects">
      {projects.map((p) => (
        <li key={p.id}>
          <ProjectCard project={p} />
        </li>
      ))}
    </ul>
  );
}

/** The placeholder shown by the dashboard while its data loads. */
export function ProjectsSkeleton() {
  return (
    <div className="grid gap-4 sm:grid-cols-2" aria-hidden>
      {[0, 1, 2, 3].map((i) => (
        <Card key={i} className="flex flex-col gap-3 p-5">
          <div className="flex items-center gap-2.5">
            <div className="bg-surface-2 size-9 rounded-lg" />
            <div className="bg-surface-2 h-4 w-40 rounded" />
          </div>
          <div className="bg-surface-2 h-4 w-full rounded" />
          <div className="bg-surface-2 h-4 w-2/3 rounded" />
        </Card>
      ))}
    </div>
  );
}
