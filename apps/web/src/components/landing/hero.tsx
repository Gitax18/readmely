import { Check, Lock, Sparkles } from "lucide-react";
import { ProductPreview } from "./product-preview";
import { RepoForm } from "./repo-form";

/** First screen: the promise, the one action (a repository address) and a picture of the result. */
export function Hero({ signedIn }: { signedIn: boolean }) {
  return (
    <section className="relative px-4 pt-14 text-center sm:px-6 sm:pt-20">
      <div
        aria-hidden
        className="bg-glow pointer-events-none absolute -top-40 left-1/2 h-[500px] w-[740px] max-w-full -translate-x-1/2"
      />
      <div className="relative mx-auto flex max-w-[900px] flex-col items-center">
        <span className="animate-fade-up border-accent/25 bg-accent-soft text-accent inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-[11px] font-bold tracking-[0.12em] uppercase">
          <Sparkles className="size-3.5" aria-hidden /> Documentation grounded in your code
        </span>
        <h1 className="animate-fade-up delay-1 text-balance mt-7 mb-5 text-[clamp(40px,6.3vw,74px)] leading-[1.04] font-semibold tracking-[-0.045em]">
          Turn a repository into docs <span className="text-accent block">developers trust.</span>
        </h1>
        <p className="animate-fade-up delay-2 text-muted-foreground max-w-[670px] text-base leading-relaxed sm:text-[17px]">
          Paste a public GitHub repository. Readmely reads the code, then writes a README, a context
          pack for AI coding tools and an architecture diagram.
        </p>
        <div className="animate-fade-up delay-3 mt-9 w-full">
          <RepoForm signedIn={signedIn} />
        </div>
        <ul className="animate-fade-up delay-4 text-muted-foreground mt-5 flex flex-wrap justify-center gap-x-6 gap-y-2 text-xs">
          <li className="flex items-center gap-1.5">
            <Check className="text-accent size-3.5" aria-hidden /> 100 credits free
          </li>
          <li className="flex items-center gap-1.5">
            <Check className="text-accent size-3.5" aria-hidden /> No credit card
          </li>
          <li className="flex items-center gap-1.5">
            <Lock className="text-accent size-3.5" aria-hidden /> Public repositories only
          </li>
        </ul>
        <a
          href="#sample"
          className="animate-fade-up delay-5 text-muted-foreground hover:text-foreground mt-5 inline-flex items-center gap-2 text-[13px] font-semibold transition-colors"
        >
          Explore a sample <span className="text-accent">↓</span>
        </a>
      </div>
      <div className="animate-fade-up delay-5 relative mt-12 sm:mt-14">
        <ProductPreview />
      </div>
      <div className="text-faint mx-auto flex max-w-[900px] flex-col items-center gap-4 py-10 text-[11px] sm:flex-row sm:justify-center sm:gap-10">
        <span className="tracking-[0.09em] uppercase">Context packs for the tools you use</span>
        <span className="text-muted-foreground flex flex-wrap justify-center gap-x-8 gap-y-2 text-xs font-bold">
          <b>Claude Code</b>
          <b>Cursor</b>
          <b>AGENTS.md</b>
        </span>
      </div>
    </section>
  );
}
