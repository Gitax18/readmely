"use client";

import { ContextPackPanel } from "@/components/context-pack-panel";
import { DiagramPanel } from "@/components/diagram-panel";
import { OutputGate } from "@/components/project/output-gate";
import { useProject } from "@/components/project/project-provider";
import { ReadmePanel } from "@/components/readme-panel";

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

export function DiagramTab() {
  const { project, api, csrf, analysisKey } = useProject();
  return (
    <OutputGate what="diagram">
      <DiagramPanel key={analysisKey} api={api} projectId={project.id} csrf={csrf} />
    </OutputGate>
  );
}
