import { ArrowRight } from "lucide-react";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { BrandMark } from "@/components/ui/logo";
import { Reveal } from "@/components/ui/motion";
import { Kicker } from "./section-intro";

export function FinalCta({ signedIn }: { signedIn: boolean }) {
  return (
    <section className="bg-navy relative overflow-hidden px-4 py-24 text-white">
      <div
        aria-hidden
        className="bg-glow pointer-events-none absolute -top-56 left-1/2 h-[420px] w-[640px] -translate-x-1/2"
      />
      <Reveal className="relative mx-auto flex max-w-xl flex-col items-center gap-4 text-center">
        <BrandMark className="size-11 border border-[#3b565c]" />
        <Kicker className="text-[#68c8bd]">Your repository can explain itself</Kicker>
        <h2 className="text-[clamp(30px,4vw,44px)] leading-[1.12] font-semibold tracking-[-0.045em]">
          Start with one GitHub link.
        </h2>
        <p className="text-navy-muted text-sm">
          Turn your code into documentation you will be proud to ship.
        </p>
        <Link
          href={signedIn ? "/dashboard" : "/login"}
          className={buttonVariants({ variant: "light", size: "lg" })}
        >
          {signedIn ? "Open dashboard" : "Try Readmely free"} <ArrowRight aria-hidden />
        </Link>
        <small className="text-xs text-[#7f9699]">100 credits included · No credit card</small>
      </Reveal>
    </section>
  );
}
