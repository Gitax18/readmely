"use client";

import { Check, FileText } from "lucide-react";
import * as React from "react";
import { Badge } from "@/components/ui/badge";
import { Reveal } from "@/components/ui/motion";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";
import { CONTEXT_FILES, DIAGRAM_NODES, KIND_STYLE, SAMPLE_REPO } from "./sample-data";
import { Kicker } from "./section-intro";

const tabClass =
  "text-navy-muted hover:text-white data-[state=active]:text-white after:bg-[#58c3b6]";

function FileBar({ name, meta, text }: { name: string; meta?: string; text: string }) {
  return (
    <div className="flex h-11 items-center justify-between border-b border-[#e2e8ea] px-4 font-mono text-[11px] text-[#738287]">
      <span>
        {name}
        {meta ? ` · ${meta}` : ""}
      </span>
      <CopyText text={text} />
    </div>
  );
}

/** A copy control for the sample; it copies the sample text, like the real one copies yours. */
function CopyText({ text }: { text: string }) {
  const [copied, setCopied] = React.useState(false);
  return (
    <button
      type="button"
      onClick={() => {
        void navigator.clipboard.writeText(text).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        });
      }}
      className="rounded border border-[#d9e1e3] bg-white px-2 py-1 font-sans text-[11px] text-[#40545a] hover:bg-[#f4f7f8]"
    >
      <span aria-live="polite">{copied ? "Copied" : "Copy"}</span>
    </button>
  );
}

const README_SOURCE = `# Streambase

A lightweight event processing toolkit for reliable, real-time applications.

## Installation

pnpm add @acme/streambase

## Quick start

import { createStream } from "@acme/streambase"

const orders = createStream<Order>("orders")`;

function SampleReadme() {
  return (
    <div className="mx-auto h-[445px] max-w-3xl overflow-hidden rounded-md bg-white text-[#19282c]">
      <FileBar name="README.md" text={README_SOURCE} />
      <article
        className="scroll-thin h-[402px] overflow-y-auto px-6 pt-8 pb-14 sm:px-14"
        tabIndex={0}
        aria-label="README.md contents"
      >
        <h3 className="text-[27px] font-semibold tracking-tight">Streambase</h3>
        <p className="mt-1 text-[13px] text-[#65757a]">
          A lightweight event processing toolkit for reliable, real-time applications.
        </p>
        <div className="my-4 flex flex-wrap gap-1.5">
          {["TypeScript", "MIT License", "build passing"].map((b) => (
            <span
              key={b}
              className="rounded bg-[#e6f4f1] px-1.5 py-0.5 font-mono text-[10px] text-[#087c72]"
            >
              {b}
            </span>
          ))}
        </div>
        <h4 className="mt-6 mb-2 text-sm font-semibold">Overview</h4>
        <p className="text-[12px] leading-relaxed text-[#65757a]">
          Streambase provides a type-safe foundation for publishing, processing and observing events
          across distributed services.
        </p>
        <h4 className="mt-6 mb-2 text-sm font-semibold">Installation</h4>
        <pre className="rounded-md bg-[#17262a] p-3 text-[11px] text-[#d9e4e3]">
          <span className="text-[#55c5b7]">$</span> pnpm add @acme/streambase
        </pre>
        <h4 className="mt-6 mb-2 text-sm font-semibold">Quick start</h4>
        <pre className="overflow-x-auto rounded-md bg-[#17262a] p-3 text-[11px] leading-relaxed text-[#d9e4e3]">
          <span className="text-[#55c5b7]">import</span> {"{ createStream }"}{" "}
          <span className="text-[#55c5b7]">from</span> &quot;@acme/streambase&quot;{"\n\n"}const
          orders = createStream&lt;Order&gt;(&quot;orders&quot;)
        </pre>
      </article>
    </div>
  );
}

function highlight(content: string) {
  return content.split("\n").map((line, i) => (
    <React.Fragment key={i}>
      {/^(#|---)/.test(line) ? <b className="font-semibold text-[#142126]">{line}</b> : line}
      {"\n"}
    </React.Fragment>
  ));
}

function SampleContext() {
  const [name, setName] = React.useState<(typeof CONTEXT_FILES)[number]["name"]>("CLAUDE.md");
  const file = CONTEXT_FILES.find((f) => f.name === name) ?? CONTEXT_FILES[0];
  return (
    <div className="mx-auto grid h-[445px] max-w-4xl grid-rows-[auto_1fr] overflow-hidden rounded-md bg-white text-[#19282c] sm:grid-cols-[190px_1fr] sm:grid-rows-1">
      <aside
        aria-label="Context files"
        className="flex gap-1 overflow-x-auto border-b border-[#e2e8ea] bg-[#f7f9f9] p-2 sm:flex-col sm:overflow-visible sm:border-r sm:border-b-0 sm:p-3"
      >
        <div className="hidden px-2 pt-1 pb-2 text-[10px] font-bold tracking-[0.1em] text-[#8a999d] uppercase sm:block">
          Context pack
        </div>
        {CONTEXT_FILES.map((f) => (
          <button
            key={f.name}
            type="button"
            onClick={() => setName(f.name)}
            aria-pressed={f.name === name}
            className={cn(
              "flex shrink-0 items-center gap-2 rounded-md p-2.5 text-left transition-colors",
              f.name === name ? "bg-[#e4efed] text-[#176e65]" : "text-[#64757a] hover:bg-[#edf2f2]",
            )}
          >
            <FileText className="size-3.5 shrink-0" aria-hidden />
            <span>
              <b className="block font-mono text-[11px] font-medium text-[#273a3f]">{f.name}</b>
              <small className="hidden text-[10px] text-[#839296] sm:block">{f.tool}</small>
            </span>
          </button>
        ))}
        <div className="mt-auto hidden items-center gap-1.5 border-t border-[#e2e8ea] px-2 pt-3 text-[11px] text-[#288272] sm:flex">
          <Check className="size-3" aria-hidden /> Free to generate
        </div>
      </aside>
      <div className="min-w-0">
        <FileBar name={file.name} meta={`about ${file.tokens} tokens`} text={file.content} />
        <pre
          tabIndex={0}
          aria-label={`${file.name} contents`}
          className="scroll-thin h-[402px] overflow-auto p-6 text-[11px] leading-[1.8] whitespace-pre-wrap text-[#394b50] sm:px-10"
        >
          <code>{highlight(file.content)}</code>
        </pre>
      </div>
    </div>
  );
}

const ZOOMS = [
  { level: 50, width: "w-[430px]" },
  { level: 75, width: "w-[645px]" },
  { level: 100, width: "w-[860px]" },
  { level: 150, width: "w-[1290px]" },
] as const;

function SampleDiagram() {
  const [zoom, setZoom] = React.useState<(typeof ZOOMS)[number]["level"]>(100);
  const width = ZOOMS.find((z) => z.level === zoom)?.width ?? "w-[860px]";
  return (
    <div className="mx-auto flex h-[445px] max-w-4xl flex-col overflow-hidden rounded-md bg-white text-[#19282c]">
      <div className="flex h-11 items-center justify-between border-b border-[#e2e8ea] px-4 font-mono text-[11px] text-[#738287]">
        <span>Streambase event processing · 6 parts</span>
        <div role="group" aria-label="Zoom" className="flex gap-1">
          {ZOOMS.map((z) => (
            <button
              key={z.level}
              type="button"
              aria-pressed={zoom === z.level}
              onClick={() => setZoom(z.level)}
              className={cn(
                "min-w-9 rounded border px-2 py-1 font-sans text-[11px] transition-colors",
                zoom === z.level
                  ? "border-[#81bdb5] bg-[#edf7f5] text-[#176e65]"
                  : "border-[#d9e1e3] bg-white text-[#40545a] hover:bg-[#f4f7f8]",
              )}
            >
              {z.level}%
            </button>
          ))}
        </div>
      </div>
      <div
        tabIndex={0}
        aria-label="Architecture diagram, scrollable"
        className="scroll-thin bg-dots flex-1 overflow-auto bg-[#fafcfc]"
      >
        <svg
          viewBox="0 0 860 360"
          role="img"
          aria-label="Diagram: a web application calls the Stream API, which publishes to an event queue and stores events in PostgreSQL. A worker takes jobs from the queue, updates status in PostgreSQL and notifies webhook targets."
          className={cn("h-auto max-w-none transition-[width] duration-300", width)}
        >
          <defs>
            <marker id="sd-arrow" markerWidth="8" markerHeight="6" refX="7" refY="3" orient="auto">
              <path d="M0,0 L8,3 L0,6" fill="none" stroke="#71878b" />
            </marker>
          </defs>
          <g fill="none" strokeWidth="1.3">
            <path d="M175 100 H280" stroke="#71878b" markerEnd="url(#sd-arrow)" />
            <path d="M440 100 H510" stroke="#71878b" markerEnd="url(#sd-arrow)" />
            <path
              d="M590 145 V240"
              stroke="#b07945"
              strokeDasharray="5 4"
              markerEnd="url(#sd-arrow)"
            />
            <path
              d="M360 145 V260 H280"
              stroke="#3e8d70"
              strokeDasharray="1.5 3"
              markerEnd="url(#sd-arrow)"
            />
            <path
              d="M510 285 H280"
              stroke="#3e8d70"
              strokeDasharray="1.5 3"
              markerEnd="url(#sd-arrow)"
            />
            <path
              d="M670 285 H780 V145"
              stroke="#b07945"
              strokeDasharray="5 4"
              markerEnd="url(#sd-arrow)"
            />
          </g>
          <g className="font-mono" fontSize="9" fill="#75878a">
            <text x="196" y="90">
              HTTPS request
            </text>
            <text x="458" y="90">
              publish
            </text>
            <text x="600" y="195">
              enqueue
            </text>
            <text x="372" y="225">
              store event
            </text>
            <text x="360" y="276">
              update status
            </text>
            <text x="700" y="275">
              notify
            </text>
          </g>
          {DIAGRAM_NODES.map((n) => {
            const s = KIND_STYLE[n.kind];
            return (
              <g key={n.title}>
                <rect
                  x={n.x}
                  y={n.y}
                  width="160"
                  height="90"
                  rx="8"
                  fill={s.fill}
                  stroke={s.stroke}
                  strokeWidth="1.3"
                />
                <text
                  x={n.x + 14}
                  y={n.y + 26}
                  fontSize="8"
                  fontWeight="700"
                  letterSpacing="1"
                  fill={s.label}
                >
                  {n.kind === "data"
                    ? "DATA STORE"
                    : n.kind === "gateway"
                      ? "GATEWAY"
                      : n.kind.toUpperCase()}
                </text>
                <text x={n.x + 14} y={n.y + 50} fontSize="13" fontWeight="600" fill="#142126">
                  {n.title}
                </text>
                <text x={n.x + 14} y={n.y + 68} fontSize="10" fill="#748589">
                  {n.tech}
                </text>
              </g>
            );
          })}
        </svg>
      </div>
      <div className="flex items-center gap-5 border-t border-[#e2e8ea] px-4 py-2.5 text-[11px] text-[#6f8084]">
        <b className="mr-auto hidden font-semibold text-[#40545a] sm:block">Connections</b>
        {[
          ["Request", "border-solid"],
          ["Async event", "border-dashed"],
          ["Data write", "border-dotted"],
        ].map(([t, b]) => (
          <span key={t} className="flex items-center gap-1.5">
            <i className={cn("w-[18px] border-t border-[#71878b]", b)} /> {t}
          </span>
        ))}
      </div>
    </div>
  );
}

/** The sample section: one example repository, read-only, with the three outputs in tabs. */
export function Sample() {
  return (
    <section id="sample" className="bg-navy px-4 py-24 text-[#f5f9f8] sm:py-28">
      <Reveal className="mx-auto mb-14 flex max-w-2xl flex-col items-center gap-4 text-center">
        <Kicker className="text-[#5dc4b7]">See the output</Kicker>
        <h2 className="text-[clamp(30px,4vw,46px)] leading-[1.12] font-semibold tracking-[-0.045em]">
          Judge the work before you sign up.
        </h2>
        <p className="text-navy-muted max-w-xl text-[15px]">
          An example of what Readmely creates for a TypeScript library. It is a sample, not a real
          repository, so you can read every output without signing in.
        </p>
      </Reveal>
      <Reveal className="mx-auto w-full max-w-[1000px]">
        <div className="overflow-hidden rounded-xl border border-[#30474c] bg-[#101c20] shadow-[0_35px_80px_rgb(0_0_0/0.24)]">
          <div className="flex items-center justify-between border-b border-[#2a3d42] px-5 py-4 sm:px-6">
            <div className="flex items-center gap-3">
              <span
                aria-hidden
                className="grid size-8 place-items-center rounded-md bg-[#233a3f] font-bold text-[#65cbbf]"
              >
                S
              </span>
              <span>
                <b className="block text-[13px]">{SAMPLE_REPO.name}</b>
                <small className="text-[11px] text-[#82979b]">{SAMPLE_REPO.facts}</small>
              </span>
            </div>
            <Badge
              tone="ok"
              dot
              className="hidden border-[#70c8ad]/30 bg-transparent text-[#70c8ad] sm:inline-flex"
            >
              Analysis ready
            </Badge>
          </div>
          <Tabs defaultValue="readme" order={["readme", "context", "diagram"]}>
            <TabsList className="border-[#2a3d42] px-5 sm:px-6">
              <TabsTrigger value="readme" className={tabClass}>
                README
              </TabsTrigger>
              <TabsTrigger value="context" className={tabClass}>
                Context pack
                <span className="rounded bg-[#183c39] px-1.5 py-0.5 text-[9px] font-bold text-[#5dc4b7]">
                  Free
                </span>
              </TabsTrigger>
              <TabsTrigger value="diagram" className={tabClass}>
                Diagram
              </TabsTrigger>
            </TabsList>
            <div className="min-h-[500px] bg-[#142227] p-3 sm:p-7">
              <TabsContent value="readme">
                <SampleReadme />
              </TabsContent>
              <TabsContent value="context">
                <SampleContext />
              </TabsContent>
              <TabsContent value="diagram">
                <SampleDiagram />
              </TabsContent>
            </div>
          </Tabs>
        </div>
      </Reveal>
    </section>
  );
}
