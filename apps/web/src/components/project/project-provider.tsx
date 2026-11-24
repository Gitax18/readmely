"use client";

import { useRouter } from "next/navigation";
import * as React from "react";
import { usePolling } from "@/components/projects-view";
import { createProjectsApi, type ProjectsApi } from "@/lib/projects-api";
import { isWorking, type Project } from "@/lib/project-schema";
import { projectFailureText } from "@/lib/project-text";

type ProjectContextValue = {
  project: Project;
  csrf: string;
  api: ProjectsApi;
  working: boolean;
  busy: boolean;
  error: string | null;
  /** A key that changes with every analysis, so panels reload what they show. */
  analysisKey: string;
  /** Bumped when the summary lands, so the context pack is rebuilt with it. */
  packVersion: number;
  bumpPack: () => void;
  analyse: () => Promise<void>;
  remove: () => Promise<void>;
  /** Turns a failed API call into a message, or sends a signed-out visitor to sign in. */
  fail: (f: Parameters<typeof projectFailureText>[0]) => void;
};

const ProjectContext = React.createContext<ProjectContextValue | null>(null);

export function useProject(): ProjectContextValue {
  const ctx = React.useContext(ProjectContext);
  if (!ctx) throw new Error("useProject must be used inside <ProjectProvider>");
  return ctx;
}

/**
 * Holds the one project every tab of the workspace shows, and keeps it live: while an analysis
 * runs it asks again every two seconds. The layout fetches the project once; tabs read it here.
 */
export function ProjectProvider({
  initial,
  csrf,
  api,
  children,
}: {
  initial: Project;
  csrf: string;
  api?: ProjectsApi;
  children: React.ReactNode;
}) {
  const router = useRouter();
  const [client] = React.useState(() => api ?? createProjectsApi());
  const [project, setProject] = React.useState<Project>(initial);
  const [error, setError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);
  const [packVersion, setPackVersion] = React.useState(0);

  const working = isWorking(project);

  const fail = React.useCallback(
    (f: Parameters<typeof projectFailureText>[0]) => {
      if (f.kind === "unauthenticated") {
        router.replace("/login");
        return;
      }
      setError(projectFailureText(f));
    },
    [router],
  );

  usePolling(working, async () => {
    const res = await client.get(project.id);
    if (res.ok) {
      setProject(res.project);
      // The analysis just settled: the credit balance in the top bar changed.
      if (!isWorking(res.project)) router.refresh();
    } else if (res.kind === "not_found") router.replace("/dashboard");
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
    fail(res);
  }

  const value: ProjectContextValue = {
    project,
    csrf,
    api: client,
    working,
    busy,
    error,
    analysisKey: project.job?.finished_at ?? project.updated_at,
    packVersion,
    bumpPack: () => setPackVersion((v) => v + 1),
    analyse,
    remove,
    fail,
  };
  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>;
}
