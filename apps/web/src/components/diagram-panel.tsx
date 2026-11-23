"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { usePolling } from "@/components/projects-view";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import type { DiagramState } from "@/lib/diagram-schema";
import { DIAGRAM_COST, ZOOM_STEPS, svgDataUrl, svgSize } from "@/lib/diagram-text";
import { diagramFailureText, diagramStepLabel, projectFailureText } from "@/lib/project-text";
import type { ProjectsApi } from "@/lib/projects-api";

type Load = { kind: "loading" } | { kind: "error" } | { kind: "ready"; state: DiagramState };

function isWorking(s: DiagramState): boolean {
  return s.job !== null && ["queued", "running"].includes(s.job.status);
}

function save(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/** The architecture diagram: draw it, look at it (zoom), download it as SVG or PNG, draw it again. */
export function DiagramPanel({
  api,
  projectId,
  csrf,
}: {
  api: ProjectsApi;
  projectId: string;
  csrf: string;
}) {
  const router = useRouter();
  const [load, setLoad] = useState<Load>({ kind: "loading" });
  const [focus, setFocus] = useState("");
  const [zoom, setZoom] = useState<number>(1);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let live = true;
    void api.diagram(projectId).then((res) => {
      if (!live) return;
      setLoad(res.ok ? { kind: "ready", state: res.state } : { kind: "error" });
    });
    return () => {
      live = false;
    };
  }, [api, projectId]);

  const state = load.kind === "ready" ? load.state : null;
  const working = state !== null && isWorking(state);

  usePolling(working, async () => {
    const res = await api.diagram(projectId);
    if (!res.ok) return;
    setLoad({ kind: "ready", state: res.state });
    if (!isWorking(res.state)) router.refresh(); // the credit balance changed
  });

  async function generate() {
    if (busy) return;
    setBusy(true);
    setError(null);
    const res = await api.generateDiagram(csrf, projectId, focus.trim());
    setBusy(false);
    if (!res.ok) {
      if (res.kind === "unauthenticated") router.replace("/login");
      else setError(projectFailureText(res));
      return;
    }
    const next = await api.diagram(projectId);
    if (next.ok) setLoad({ kind: "ready", state: next.state });
    router.refresh();
  }

  function downloadSvg() {
    if (state?.diagram)
      save(new Blob([state.diagram.svg], { type: "image/svg+xml" }), "architecture.svg");
  }

  function downloadPng() {
    const d = state?.diagram;
    const size = d ? svgSize(d.svg) : null;
    if (!d || !size) return;
    const img = new Image();
    img.onload = () => {
      const scale = 2;
      const canvas = document.createElement("canvas");
      canvas.width = size.width * scale;
      canvas.height = size.height * scale;
      const ctx = canvas.getContext("2d");
      if (!ctx) return;
      ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
      canvas.toBlob((blob) => {
        if (blob) save(blob, "architecture.png");
      }, "image/png");
    };
    img.src = svgDataUrl(d.svg);
  }

  if (load.kind === "loading") {
    return <p className="text-muted-foreground text-sm">Loading the diagram…</p>;
  }
  if (load.kind === "error" || state === null) {
    return (
      <p className="text-muted-foreground text-sm">
        We could not load the diagram. Try reloading the page.
      </p>
    );
  }

  const job = state.job;
  const d = state.diagram;
  const size = d ? svgSize(d.svg) : null;

  return (
    <div className="flex flex-col gap-4">
      {working && job ? (
        <div className="flex flex-col gap-2" aria-live="polite">
          <Progress value={job.progress} label="Diagram progress" />
          <span className="text-sm">{diagramStepLabel(job.step)}…</span>
        </div>
      ) : null}
      {job && job.status === "failed" ? (
        <p role="status" className="text-muted-foreground text-sm">
          {diagramFailureText(job.error_code)}
        </p>
      ) : null}

      {d ? (
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium">{d.title}</span>
            <span className="text-muted-foreground text-xs">{d.node_count} parts</span>
          </div>
          <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Zoom">
            {ZOOM_STEPS.map((z) => (
              <Button
                key={z}
                variant={z === zoom ? "primary" : "secondary"}
                className="h-8 px-3"
                aria-pressed={z === zoom}
                onClick={() => setZoom(z)}
              >
                {Math.round(z * 100)}%
              </Button>
            ))}
          </div>
          <div
            className="border-border max-h-[32rem] overflow-auto rounded-md border bg-white"
            role="region"
            aria-label="Architecture diagram"
            tabIndex={0}
          >
            {/* An img cannot run scripts, so even a surprising SVG could not act on the page. */}
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={svgDataUrl(d.svg)}
              alt={d.title}
              width={size ? size.width * zoom : undefined}
              height={size ? size.height * zoom : undefined}
              style={{ maxWidth: "none" }}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" onClick={downloadSvg}>
              Download SVG
            </Button>
            <Button variant="secondary" onClick={downloadPng} disabled={!size}>
              Download PNG
            </Button>
          </div>
        </div>
      ) : null}

      {!working ? (
        <div className="flex flex-col gap-2">
          <label htmlFor="diagram-focus" className="text-sm">
            {d ? "Draw it again" : "Anything it should emphasise?"}{" "}
            <span className="text-muted-foreground text-xs">
              Optional, for example &quot;the background worker&quot;.
            </span>
          </label>
          <Input
            id="diagram-focus"
            value={focus}
            maxLength={200}
            onChange={(e) => setFocus(e.target.value)}
          />
          <div className="flex flex-wrap items-center gap-3">
            <Button onClick={() => void generate()} disabled={busy}>
              {d ? "Draw a new diagram" : "Draw my diagram"}
            </Button>
            <span className="text-muted-foreground text-xs">
              Up to {DIAGRAM_COST} credits; you pay what it costs and the rest is returned.
            </span>
          </div>
        </div>
      ) : null}
      {error ? (
        <p role="alert" className="text-bad text-sm">
          {error}
        </p>
      ) : null}
    </div>
  );
}
