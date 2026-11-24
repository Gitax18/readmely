import { Badge } from "@/components/ui/badge";

/** The hero picture: a README beside an architecture diagram, in a window frame. Illustrative. */
export function ProductPreview() {
  return (
    <div
      role="img"
      aria-label="Example of a generated README beside an architecture diagram, for a sample repository"
      className="border-border bg-card shadow-lift mx-auto w-full max-w-[1050px] overflow-hidden rounded-t-[13px] border border-b-0 text-left"
    >
      <div className="border-border text-muted-foreground flex h-12 items-center gap-5 border-b px-4 text-[11px]">
        <span className="flex gap-1.5" aria-hidden>
          <i className="bg-border-strong size-2 rounded-full" />
          <i className="bg-border-strong size-2 rounded-full" />
          <i className="bg-border-strong size-2 rounded-full" />
        </span>
        <span>acme / streambase</span>
        <Badge tone="ok" dot className="ml-auto">
          Ready
        </Badge>
      </div>
      <div className="border-border text-muted-foreground flex h-11 items-end gap-6 border-b px-6 text-[11px] font-medium">
        <span className="text-foreground border-accent flex h-11 items-center border-b-2">
          README
        </span>
        <span className="flex h-11 items-center">Context pack</span>
        <span className="hidden h-11 items-center sm:flex">Diagram</span>
      </div>
      <div className="bg-surface-2 grid gap-4 p-4 sm:p-7 md:min-h-[420px] md:grid-cols-[1.06fr_.94fr]">
        <div className="border-border bg-card rounded-lg border p-6 sm:px-8">
          <div className="text-faint mb-5 font-mono text-[10px]">README.md</div>
          <h3 className="text-2xl font-semibold tracking-tight">Streambase</h3>
          <p className="text-muted-foreground mt-1 max-w-sm text-[12px]">
            A lightweight event processing toolkit for reliable, real-time applications.
          </p>
          <div className="my-4 flex flex-wrap gap-1.5">
            {["TypeScript", "MIT", "build passing"].map((b) => (
              <span
                key={b}
                className="bg-accent-soft text-accent rounded px-1.5 py-0.5 font-mono text-[9px]"
              >
                {b}
              </span>
            ))}
          </div>
          <h4 className="mt-5 mb-2 text-[13px] font-semibold">Getting started</h4>
          <pre className="bg-code text-code-foreground rounded-md p-3 text-[10px] leading-relaxed">
            <span className="text-code-accent">$</span> pnpm install{"\n"}
            <span className="text-code-accent">$</span> pnpm dev
          </pre>
          <h4 className="mt-5 mb-1 text-[13px] font-semibold">What’s included</h4>
          <ul className="text-muted-foreground list-disc pl-4 text-[11px] leading-6">
            <li>Typed event streams</li>
            <li>Durable worker queues</li>
          </ul>
        </div>
        <div className="border-border bg-card hidden rounded-lg border p-5 md:block">
          <div className="text-muted-foreground flex justify-between text-[11px]">
            <span>Architecture</span>
            <b className="text-faint font-medium">4 parts</b>
          </div>
          <svg viewBox="0 0 330 300" className="mt-3 h-auto w-full" aria-hidden>
            <defs>
              <marker
                id="pv-arrow"
                markerWidth="8"
                markerHeight="6"
                refX="7"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L8,3 L0,6" fill="none" stroke="currentColor" />
              </marker>
            </defs>
            <g
              className="text-muted-foreground"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.2"
            >
              <path d="M128 60 H202" markerEnd="url(#pv-arrow)" />
              <path d="M245 90 V160" strokeDasharray="5 4" markerEnd="url(#pv-arrow)" />
              <path d="M200 215 H128" strokeDasharray="1.5 3" markerEnd="url(#pv-arrow)" />
            </g>
            <text x="165" y="50" textAnchor="middle" className="fill-faint font-mono" fontSize="9">
              request
            </text>
            {(
              [
                [8, 36, "Web client", "React", "var(--kind-client)"],
                [204, 36, "API service", "Node.js", "var(--kind-service)"],
                [204, 186, "Worker", "BullMQ", "var(--kind-worker)"],
                [8, 186, "PostgreSQL", "Data store", "var(--kind-data)"],
              ] as const
            ).map(([x, y, name, tech, color]) => (
              <g key={name}>
                <rect
                  x={x}
                  y={y}
                  width="118"
                  height="52"
                  rx="7"
                  className="fill-card"
                  stroke={color}
                  strokeWidth="1.3"
                />
                <text
                  x={x + 12}
                  y={y + 23}
                  className="fill-foreground"
                  fontSize="11"
                  fontWeight="600"
                >
                  {name}
                </text>
                <text x={x + 12} y={y + 39} className="fill-muted-foreground" fontSize="9">
                  {tech}
                </text>
              </g>
            ))}
          </svg>
        </div>
      </div>
    </div>
  );
}
