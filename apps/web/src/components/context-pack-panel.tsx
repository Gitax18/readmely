"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
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
  const [copied, setCopied] = useState(false);

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
    return <p className="text-muted-foreground text-sm">Building the pack…</p>;
  }
  if (state.kind === "error") {
    return (
      <p className="text-muted-foreground text-sm">
        We could not build the context pack. Try reloading the page.
      </p>
    );
  }
  const { pack } = state;
  const preset = pack.presets.find((p) => p.id === presetId) ?? pack.presets[0];
  if (!preset) return null;
  const text = preset.files.map((f) => f.content).join("\n");

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

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
    <div className="flex flex-col gap-4">
      <fieldset className="flex flex-wrap gap-2">
        <legend className="sr-only">Which tool is the pack for</legend>
        {pack.presets.map((p) => (
          <label
            key={p.id}
            className={`border-border cursor-pointer rounded-md border px-3 py-1.5 text-sm ${
              p.id === preset.id ? "bg-foreground text-background" : ""
            }`}
          >
            <input
              type="radio"
              name="context-pack-preset"
              value={p.id}
              checked={p.id === preset.id}
              onChange={() => setPresetId(p.id)}
              className="sr-only"
            />
            {p.label}
          </label>
        ))}
      </fieldset>
      <p className="text-muted-foreground text-xs">
        {preset.description} About {formatCount(preset.tokens)} tokens.
        {pack.with_summary ? "" : " The written summary is not included yet."}
      </p>
      <pre
        aria-label={`Preview of ${preset.files.map((f) => f.path).join(", ")}`}
        className="border-border max-h-96 overflow-auto rounded-md border p-3 text-xs whitespace-pre-wrap"
      >
        {text}
      </pre>
      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={download}>Download ZIP</Button>
        <Button variant="secondary" onClick={() => void copy()}>
          {copied ? "Copied" : "Copy"}
        </Button>
        <span className="text-muted-foreground text-xs">
          Unzip it at the root of the repository.
        </span>
      </div>
    </div>
  );
}
