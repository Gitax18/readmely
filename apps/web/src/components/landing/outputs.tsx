import { Check, Code2, FileText, Network } from "lucide-react";
import { Reveal } from "@/components/ui/motion";
import { cn } from "@/lib/utils";
import { SectionIntro } from "./section-intro";

function FeatureIcon({ children }: { children: React.ReactNode }) {
  return (
    <div className="bg-accent-soft text-accent mb-5 grid size-10 place-items-center rounded-lg [&>svg]:size-5">
      {children}
    </div>
  );
}

function Label({ children }: { children: React.ReactNode }) {
  return <span className="text-faint text-[10px] font-bold tracking-[0.12em]">{children}</span>;
}

function Card({ className, children }: { className?: string; children: React.ReactNode }) {
  return (
    <article
      className={cn(
        "border-border bg-card shadow-soft hover:border-border-strong relative overflow-hidden rounded-card border p-8 transition-colors sm:p-9",
        className,
      )}
    >
      {children}
    </article>
  );
}

export function Outputs() {
  return (
    <section className="border-border mx-auto w-full max-w-[1100px] border-t px-4 py-24 sm:px-6 sm:py-28">
      <SectionIntro
        kicker="Three useful outputs"
        title={<>Everything your repository needs to explain itself.</>}
        align="left"
      />
      <div className="grid gap-5 md:grid-cols-2">
        <Reveal className="md:col-span-2">
          <Card className="grid items-center gap-10 md:grid-cols-[0.85fr_1.15fr] md:gap-14">
            <div>
              <FeatureIcon>
                <FileText aria-hidden />
              </FeatureIcon>
              <Label>README</Label>
              <h3 className="mt-2 mb-3 text-2xl leading-tight font-semibold tracking-tight">
                Documentation that starts strong.
              </h3>
              <p className="text-muted-foreground mb-4 max-w-md text-[13px]">
                Choose a layout, edit in Markdown with a live preview, rewrite one section at a
                time, and restore any earlier version.
              </p>
              <ul className="text-muted-foreground grid gap-2 text-xs">
                {["Live Markdown editor", "Ten-point health check", "Full version history"].map(
                  (t) => (
                    <li key={t} className="flex items-center gap-2">
                      <Check className="text-accent size-3.5" aria-hidden /> {t}
                    </li>
                  ),
                )}
              </ul>
            </div>
            <div
              aria-hidden
              className="border-border bg-surface-2 shadow-lift overflow-hidden rounded-xl border md:-rotate-1"
            >
              <div className="border-border text-muted-foreground flex h-10 items-center gap-1.5 border-b px-3 text-[10px]">
                <i className="bg-border-strong size-1.5 rounded-full" />
                <i className="bg-border-strong size-1.5 rounded-full" />
                <span className="ml-2">README.md</span>
                <b className="text-accent ml-auto font-semibold">Saved</b>
              </div>
              <div className="border-border bg-card m-3.5 grid gap-3.5 rounded-md border p-6">
                <span className="font-mono text-[19px]"># Streambase</span>
                <span className="bg-border block h-2 w-4/5 rounded" />
                <span className="mt-2 font-mono text-[13px]">## Getting started</span>
                <span className="bg-code text-code-accent rounded px-2.5 py-2 font-mono text-[10px]">
                  pnpm install
                </span>
                <span className="bg-border block h-2 w-1/2 rounded" />
              </div>
              <div className="text-muted-foreground grid grid-cols-[1fr_auto] gap-2 px-4 pb-4 text-[10px]">
                <span>README health</span>
                <b className="text-accent">90 / 100</b>
                <div className="bg-border col-span-2 h-1 overflow-hidden rounded">
                  <div className="bg-accent h-full w-[90%]" />
                </div>
              </div>
            </div>
          </Card>
        </Reveal>

        <Reveal delay={0.05}>
          <Card className="h-full">
            <FeatureIcon>
              <Code2 aria-hidden />
            </FeatureIcon>
            <Label>CONTEXT PACK</Label>
            <h3 className="mt-2 mb-3 text-2xl leading-tight font-semibold tracking-tight">
              Give coding tools the full picture.
            </h3>
            <p className="text-muted-foreground mb-8 max-w-md text-[13px]">
              Instant, structured context for Claude Code, Cursor, AGENTS.md or any AI coding tool,
              built from the analysis.
            </p>
            <div aria-hidden className="relative mx-auto h-[150px] w-4/5">
              {[
                ["CLAUDE.md", "Claude Code", "top-0 z-30"],
                ["AGENTS.md", "Open standard", "top-[44px] z-20 scale-95 opacity-85"],
                ["readmely.mdc", "Cursor rule", "top-[88px] z-10 scale-90 opacity-65"],
              ].map(([name, meta, pos]) => (
                <div
                  key={name}
                  className={cn(
                    "border-border bg-card shadow-soft absolute inset-x-0 h-[58px] rounded-lg border px-4 py-3",
                    pos,
                  )}
                >
                  <b className="block font-mono text-[11px] font-medium">{name}</b>
                  <small className="text-faint mt-0.5 block font-mono text-[9px]">{meta}</small>
                </div>
              ))}
            </div>
            <p className="text-accent mt-4 text-center text-[11px] font-semibold">
              Free · no credits
            </p>
          </Card>
        </Reveal>

        <Reveal delay={0.1}>
          <Card className="h-full">
            <FeatureIcon>
              <Network aria-hidden />
            </FeatureIcon>
            <Label>ARCHITECTURE DIAGRAM</Label>
            <h3 className="mt-2 mb-3 text-2xl leading-tight font-semibold tracking-tight">
              See how the pieces fit together.
            </h3>
            <p className="text-muted-foreground mb-8 max-w-md text-[13px]">
              A clean diagram of the services, workers, stores and connections found in your code,
              ready to drop into a README or a portfolio.
            </p>
            <div
              aria-hidden
              className="relative flex h-[130px] items-center justify-center gap-2 text-[10px]"
            >
              {[
                ["Client", "client"],
                ["API", "service"],
                ["Worker", "worker"],
              ].map(([t, k], i) => (
                <div key={t} className="flex items-center gap-2">
                  {i > 0 ? <span className="text-faint">{i === 1 ? "→" : "⇢"}</span> : null}
                  <div
                    className={cn(
                      "min-w-[72px] rounded-md border px-3 py-3 text-center",
                      k === "client" && "border-kind-client bg-kind-client/10",
                      k === "service" && "border-kind-service bg-kind-service/10",
                      k === "worker" && "border-kind-worker bg-kind-worker/10",
                    )}
                  >
                    {t}
                  </div>
                </div>
              ))}
              <div className="border-kind-data bg-kind-data/10 absolute right-[16%] bottom-0 min-w-[72px] rounded-md border px-3 py-3 text-center">
                Database
              </div>
            </div>
            <p className="text-accent mt-4 text-center text-[11px] font-semibold">
              SVG and PNG download
            </p>
          </Card>
        </Reveal>
      </div>
    </section>
  );
}
