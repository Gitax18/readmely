"use client";

import { useState } from "react";
import { ContextPackPanel } from "@/components/context-pack-panel";
import { DiagramPanel } from "@/components/diagram-panel";
import { OutputGate } from "@/components/project/output-gate";
import { useProject } from "@/components/project/project-provider";
import { ReadmePanel } from "@/components/readme-panel";
import { DIAGRAM_KINDS, KIND_TEXT, type DiagramKind } from "@/lib/diagram-text";
import { cn } from "@/lib/utils";

/** The README tab. Reloads its content whenever the project is analysed again. */
export function ReadmeTab() {
  const { project, api, csrf, analysisKey } = useProject();
  return (
    <OutputGate what="README">
      <ReadmePanel
        key={analysisKey}
        api={api}
        projectId={project.id}
        csrf={csrf}
        repoName={project.name}
      />
    </OutputGate>
  );
}

/** The context pack tab. Rebuilt when the summary lands, so the pack includes it. */
export function ContextPackTab() {
  const { project, api, analysisKey, packVersion } = useProject();
  return (
    <OutputGate what="context pack">
      <ContextPackPanel
        key={`${analysisKey}:${packVersion}`}
        api={api}
        projectId={project.id}
        repoName={project.name}
      />
    </OutputGate>
  );
}

/** The Diagram tab: a project has an architecture diagram and a database (ER) diagram, one of each. */
export function DiagramTab() {
  const { project, api, csrf, analysisKey } = useProject();
  const [kind, setKind] = useState<DiagramKind>("architecture");
  return (
    <OutputGate what="diagram">
      <div className="flex flex-col gap-6">
        <div
          role="group"
          aria-label="Kind of diagram"
          className="border-border bg-surface-2 flex w-fit gap-0.5 rounded-control border p-0.5"
        >
          {DIAGRAM_KINDS.map((k) => (
            <button
              key={k}
              type="button"
              aria-pressed={k === kind}
              onClick={() => setKind(k)}
              className={cn(
                "rounded-md px-3 py-1.5 text-sm font-semibold transition-colors",
                k === kind ? "bg-card shadow-soft" : "text-muted-foreground hover:text-foreground",
              )}
            >
              {KIND_TEXT[k].tab}
            </button>
          ))}
        </div>
        <DiagramPanel
          key={`${analysisKey}-${kind}`}
          api={api}
          projectId={project.id}
          csrf={csrf}
          kind={kind}
        />
      </div>
    </OutputGate>
  );
}
