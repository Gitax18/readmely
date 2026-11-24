"use client";

import { Download, FileText } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyButton } from "@/components/ui/copy-button";
import { Message } from "@/components/ui/message";
import { RadioCard, RadioCardGroup } from "@/components/ui/radio-cards";
import { Skeleton } from "@/components/ui/skeleton";
import type { ContextPack, ContextPackPreset } from "@/lib/context-pack-schema";
import type { ProjectsApi } from "@/lib/projects-api";
import { formatCount } from "@/lib/project-text";
import { makeZip } from "@/lib/zip";

type State = { kind: "loading" } | { kind: "error" } | { kind: "ready"; pack: ContextPack };

/** The context pack: pick a tool, read it, copy it or download it as a ZIP to unzip at the repository root. */
export function ContextPackPanel({
  api,
  projectId,
  repoName,
}: {
  api: ProjectsApi;
  projectId: string;
  repoName: string;
}) {
  const [state, setState] = useState<State>({ kind: "loading" });
  const [presetId, setPresetId] = useState<ContextPackPreset["id"]>("claude");

  useEffect(() => {
    let live = true;
    void api.contextPack(projectId).then((res) => {
      if (!live) return;
      setState(res.ok ? { kind: "ready", pack: res.pack } : { kind: "error" });
    });
    return () => {
      live = false;
    };
  }, [api, projectId]);

  if (state.kind === "loading") {
    return (
      <div
        role="status"
        aria-label="Building the pack"
        className="grid gap-4 lg:grid-cols-[260px_1fr]"
      >
        <Skeleton className="h-64 rounded-card" />
        <Skeleton className="h-[26rem] rounded-card" />
      </div>
    );
  }
  if (state.kind === "error") {
    return (
      <Message tone="error">We could not build the context pack. Try reloading the page.</Message>
    );
  }
  const { pack } = state;
  const preset = pack.presets.find((p) => p.id === presetId) ?? pack.presets[0];
  if (!preset) return null;
  const text = preset.files.map((f) => f.content).join("\n");
  const paths = preset.files.map((f) => f.path).join(", ");

  function download() {
    const zip = makeZip(preset!.files);
    const url = URL.createObjectURL(new Blob([zip], { type: "application/zip" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `${repoName.replace(/[^A-Za-z0-9._-]/g, "-")}-${preset!.id}-context.zip`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="flex flex-col gap-5">
      <p className="text-muted-foreground max-w-prose text-sm">
        Files that tell an AI coding tool what this repository is and how to work in it. Free: built
        from the analysis, no credits.
      </p>
      <div className="grid gap-5 lg:grid-cols-[260px_1fr]">
        <RadioCardGroup
          aria-label="Which tool is the pack for"
          value={preset.id}
          onValueChange={(v) => setPresetId(v as ContextPackPreset["id"])}
          className="content-start gap-2"
        >
          {pack.presets.map((p) => (
            <RadioCard
              key={p.id}
              value={p.id}
              title={p.label}
              hint={p.files.map((f) => f.path).join(", ")}
            />
          ))}
        </RadioCardGroup>

        <Card className="flex min-w-0 flex-col overflow-hidden">
          <div className="border-border flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
            <span className="flex min-w-0 items-center gap-2 font-mono text-xs">
              <FileText className="text-accent size-4 shrink-0" aria-hidden />
              <span className="truncate">{paths}</span>
            </span>
            <span className="text-muted-foreground text-xs">
              About {formatCount(preset.tokens)} tokens
            </span>
          </div>
          <pre
            tabIndex={0}
            aria-label={`Preview of ${paths}`}
            className="scroll-thin max-h-[28rem] min-h-72 flex-1 overflow-auto p-5 font-mono text-xs leading-relaxed whitespace-pre-wrap"
          >
            {text}
          </pre>
          <div className="border-border bg-surface-2 flex flex-wrap items-center gap-3 border-t px-4 py-3">
            <Button variant="accent" onClick={download}>
              <Download aria-hidden /> Download ZIP
            </Button>
            <CopyButton text={text} />
            <span className="text-muted-foreground text-xs">
              Unzip it at the root of the repository.
            </span>
          </div>
        </Card>
      </div>
      <p className="text-muted-foreground text-xs">
        {preset.description}
        {pack.with_summary ? "" : " The written summary is not included yet."}
      </p>
    </div>
  );
}
