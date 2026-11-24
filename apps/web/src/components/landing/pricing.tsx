import { ArrowRight, Code2, FileText, Network, Sparkles } from "lucide-react";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { Reveal } from "@/components/ui/motion";
import { cn } from "@/lib/utils";
import { Kicker } from "./section-intro";

const COSTS = [
  {
    icon: <Sparkles aria-hidden />,
    label: "Analyse a project",
    amount: "10 held",
    note: "Usually settles to about 1 credit",
  },
  {
    icon: <FileText aria-hidden />,
    label: "Write a README",
    amount: "15 held",
    note: "You pay what it actually cost",
  },
  {
    icon: <FileText aria-hidden />,
    label: "Rewrite one section",
    amount: "2 held",
    note: "You pay what it actually cost",
  },
  {
    icon: <Network aria-hidden />,
    label: "Draw a diagram",
    amount: "15 held",
    note: "You pay what it actually cost",
  },
  {
    icon: <Code2 aria-hidden />,
    label: "Build a context pack",
    amount: "Free",
    note: "No AI call",
    free: true,
  },
] as const;

export function Pricing({ signedIn }: { signedIn: boolean }) {
  return (
    <section id="pricing" className="mx-auto w-full max-w-[1100px] px-4 pb-24 sm:px-6 sm:pb-28">
      <Reveal className="border-border bg-card shadow-soft grid gap-12 rounded-card border p-7 sm:p-12 md:grid-cols-2 md:gap-20">
        <div className="flex flex-col items-start gap-4">
          <Kicker>Simple to start</Kicker>
          <h2 className="text-[clamp(30px,4vw,40px)] leading-[1.12] font-semibold tracking-[-0.045em]">
            100 free credits. About two full projects.
          </h2>
          <p className="text-muted-foreground max-w-md text-[13px]">
            Every new account starts with 100 credits. Readmely holds the most a job could cost,
            charges only what it used and returns the rest. A job that fails is refunded in full.
            Reading, editing and downloading never cost credits.
          </p>
          <Link
            href={signedIn ? "/dashboard" : "/login"}
            className={buttonVariants({ variant: "primary", size: "lg" })}
          >
            {signedIn ? "Open dashboard" : "Start for free"} <ArrowRight aria-hidden />
          </Link>
          <small className="text-faint text-xs">
            Paid plans are coming. No credit card needed.
          </small>
        </div>
        <dl className="border-border divide-border divide-y self-center rounded-lg border px-5">
          {COSTS.map((c) => (
            <div key={c.label} className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-0.5 py-4">
              <dt className="flex items-center gap-2.5 text-[13px] font-semibold [&>svg]:size-4 [&>svg]:text-accent">
                {c.icon}
                {c.label}
              </dt>
              <dd
                className={cn(
                  "font-mono text-xs font-medium",
                  "free" in c && c.free && "text-accent",
                )}
              >
                {c.amount}
              </dd>
              <dd className="text-faint col-span-2 pl-[26px] text-[11px]">{c.note}</dd>
            </div>
          ))}
        </dl>
      </Reveal>
    </section>
  );
}
