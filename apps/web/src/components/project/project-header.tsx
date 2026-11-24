"use client";

import {
  ChevronLeft,
  ExternalLink,
  FileText,
  LayoutGrid,
  Network,
  Settings,
  Sparkles,
} from "lucide-react";
import Link from "next/link";
import { useProject } from "@/components/project/project-provider";
import { Badge } from "@/components/ui/badge";
import { NavTabs } from "@/components/ui/tabs";
import { isWorking } from "@/lib/project-schema";
import { repoUrl, statusBadge } from "@/lib/project-text";

/** The header every tab of a project shares: the repository, its state, and the tab bar. */
export function ProjectHeader() {
  const { project } = useProject();
  const badge = statusBadge(project);
  const link = repoUrl(project);
  const base = `/projects/${project.id}`;
  return (
    <div className="flex flex-col gap-4">
      <Link
        href="/dashboard"
        className="text-muted-foreground hover:text-foreground -ml-1 inline-flex w-fit items-center gap-0.5 text-[13px] font-medium transition-colors"
      >
        <ChevronLeft className="size-4" aria-hidden /> Projects
      </Link>
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="flex min-w-0 flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight break-all sm:text-[28px]">
            {project.name}
          </h1>
          {link ? (
            <a
              href={link}
              target="_blank"
              rel="noreferrer noopener"
              className="text-muted-foreground hover:text-accent inline-flex w-fit items-center gap-1.5 text-[13px] transition-colors"
            >
              View on GitHub <ExternalLink className="size-3.5" aria-hidden />
            </a>
          ) : null}
        </div>
        <Badge tone={badge.tone} dot={isWorking(project) ? "pulse" : true} className="mt-1.5">
          {badge.label}
        </Badge>
      </div>
      <NavTabs
        label="Project"
        base={base}
        tabs={[
          { href: base, label: "Overview", icon: <LayoutGrid aria-hidden /> },
          { href: `${base}/readme`, label: "README", icon: <FileText aria-hidden /> },
          { href: `${base}/context-pack`, label: "Context pack", icon: <Sparkles aria-hidden /> },
          { href: `${base}/diagram`, label: "Diagram", icon: <Network aria-hidden /> },
          { href: `${base}/settings`, label: "Settings", icon: <Settings aria-hidden /> },
        ]}
      />
    </div>
  );
}
