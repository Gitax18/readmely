"use client";

import { Download, Maximize2, Network, Sparkles } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { usePolling } from "@/components/projects-view";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Message } from "@/components/ui/message";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import type { DiagramState } from "@/lib/diagram-schema";
import { DIAGRAM_COST, ZOOM_STEPS, svgDataUrl, svgSize } from "@/lib/diagram-text";
import { diagramFailureText, diagramStepLabel, projectFailureText } from "@/lib/project-text";
import type { ProjectsApi } from "@/lib/projects-api";
import { cn } from "@/lib/utils";

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

/** The architecture diagram: draw it, look at it (zoom, fit, drag to pan), download it, draw it again. */
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
  const [dragging, setDragging] = useState(false);
  const viewport = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x: number; y: number; left: number; top: number } | null>(null);

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

  /** Scales the drawing so all of it fits the frame (never larger than its own size). */
  function fit(size: { width: number; height: number } | null) {
    const el = viewport.current;
    if (!el || !size) return;
    const z = Math.min(el.clientWidth / size.width, el.clientHeight / size.height, 1);
    setZoom(Math.max(0.1, Math.round(z * 100) / 100));
  }

  function onPointerDown(e: React.PointerEvent<HTMLDivElement>) {
    const el = viewport.current;
    if (!el || e.button !== 0) return;
    drag.current = { x: e.clientX, y: e.clientY, left: el.scrollLeft, top: el.scrollTop };
    el.setPointerCapture(e.pointerId);
    setDragging(true);
  }
  function onPointerMove(e: React.PointerEvent<HTMLDivElement>) {
    const el = viewport.current;
    const d = drag.current;
    if (!el || !d) return;
    el.scrollLeft = d.left - (e.clientX - d.x);
    el.scrollTop = d.top - (e.clientY - d.y);
  }
  function onPointerUp() {
    drag.current = null;
    setDragging(false);
  }

  if (load.kind === "loading") {
    return (
      <div role="status" aria-label="Loading the diagram" className="flex flex-col gap-4">
        <Skeleton className="h-[26rem] rounded-card" />
      </div>
    );
  }
  if (load.kind === "error" || state === null) {
    return <Message tone="error">We could not load the diagram. Try reloading the page.</Message>;
  }

  const job = state.job;
  const d = state.diagram;
  const size = d ? svgSize(d.svg) : null;

  return (
    <div className="flex flex-col gap-6">
      {working && job ? (
        <Card aria-live="polite">
          <CardContent className="flex flex-col gap-3 pt-6">
            <Progress value={job.progress} label="Diagram progress" />
            <span className="text-muted-foreground text-sm">{diagramStepLabel(job.step)}…</span>
          </CardContent>
        </Card>
      ) : null}
      {job && job.status === "failed" ? (
        <Message tone="info">{diagramFailureText(job.error_code)}</Message>
      ) : null}

      {d ? (
        <Card className="overflow-hidden">
          <div className="border-border flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span className="text-sm font-semibold">{d.title}</span>
              <span className="text-muted-foreground text-xs">{d.node_count} parts</span>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <div
                role="group"
                aria-label="Zoom"
                className="border-border bg-surface-2 flex gap-0.5 rounded-control border p-0.5"
              >
                {ZOOM_STEPS.map((z) => (
                  <button
                    key={z}
                    type="button"
                    aria-pressed={z === zoom}
                    onClick={() => setZoom(z)}
                    className={cn(
                      "min-w-10 rounded-md px-2 py-1 text-xs font-semibold transition-colors",
                      z === zoom
                        ? "bg-card shadow-soft"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {Math.round(z * 100)}%
                  </button>
                ))}
              </div>
              <Button variant="secondary" size="sm" disabled={!size} onClick={() => fit(size)}>
                <Maximize2 aria-hidden /> Fit
              </Button>
            </div>
          </div>
          {/* The diagram stays on a light surface in both themes: its colours are made for white. */}
          <div
            ref={viewport}
            role="region"
            aria-label="Architecture diagram"
            tabIndex={0}
            onPointerDown={onPointerDown}
            onPointerMove={onPointerMove}
            onPointerUp={onPointerUp}
            onPointerCancel={onPointerUp}
            className={cn(
              "bg-dots scroll-thin max-h-[34rem] touch-pan-x touch-pan-y overflow-auto bg-white select-none",
              dragging ? "cursor-grabbing" : "cursor-grab",
            )}
          >
            {/* An img cannot run scripts, so even a surprising SVG could not act on the page. */}
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={svgDataUrl(d.svg)}
              alt={d.title}
              width={size ? size.width * zoom : undefined}
              height={size ? size.height * zoom : undefined}
              draggable={false}
              className="block max-w-none"
            />
          </div>
          <div className="border-border bg-surface-2 flex flex-wrap items-center gap-2 border-t px-4 py-3">
            <Button variant="secondary" onClick={downloadSvg}>
              <Download aria-hidden /> Download SVG
            </Button>
            <Button variant="secondary" onClick={downloadPng} disabled={!size}>
              <Download aria-hidden /> Download PNG
            </Button>
            <span className="text-faint ml-auto text-xs">Scroll or drag to move around.</span>
          </div>
        </Card>
      ) : null}

      {!working ? (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <Network className="text-accent size-4" aria-hidden />
              {d ? "Draw a new diagram" : "Draw your architecture diagram"}
            </CardTitle>
            <CardDescription>
              A picture of the main parts and how they talk to each other, drawn from the analysis.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Field
              label={d ? "Draw it again" : "Anything it should emphasise?"}
              hint={'Optional, for example "the background worker".'}
            >
              {(field) => (
                <Input
                  {...field}
                  value={focus}
                  maxLength={200}
                  onChange={(e) => setFocus(e.target.value)}
                />
              )}
            </Field>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
              <Button variant="accent" size="lg" busy={busy} onClick={() => void generate()}>
                <Sparkles aria-hidden />
                {d ? "Draw a new diagram" : "Draw my diagram"}
              </Button>
              <span className="text-muted-foreground text-xs">
                Up to {DIAGRAM_COST} credits; you pay what it costs and the rest is returned.
              </span>
            </div>
          </CardContent>
        </Card>
      ) : null}
      {error ? <Message tone="error">{error}</Message> : null}
    </div>
  );
}
