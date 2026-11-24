import { Code2, FileText } from "lucide-react";
import { GitHubMark } from "@/components/ui/brand-icons";
import { Reveal } from "@/components/ui/motion";
import { SectionIntro } from "./section-intro";

const STEPS = [
  {
    n: "01",
    icon: <GitHubMark width={22} height={22} />,
    title: "Paste a repository link",
    text: "Share any public GitHub repository. Readmely checks it exists before it starts.",
  },
  {
    n: "02",
    icon: <Code2 className="size-[22px]" aria-hidden />,
    title: "Readmely reads the code",
    text: "It maps languages, structure, entry points, commands, services and key files.",
  },
  {
    n: "03",
    icon: <FileText className="size-[22px]" aria-hidden />,
    title: "Edit and download",
    text: "Review everything, make it yours, and download files ready for your repository.",
  },
];

export function HowItWorks() {
  return (
    <section id="how" className="mx-auto w-full max-w-[1100px] px-4 py-24 sm:px-6 sm:py-28">
      <SectionIntro
        kicker="How it works"
        title={<>From repository to ready to use in about a minute.</>}
      >
        No setup, no templates to fill in, no questionnaire. The code already holds the answers.
      </SectionIntro>
      <ol className="relative grid gap-12 md:grid-cols-3 md:gap-4">
        <div
          aria-hidden
          className="border-border-strong absolute inset-x-[16%] top-[54px] hidden border-t border-dashed md:block"
        />
        {STEPS.map((s, i) => (
          <Reveal
            as="li"
            key={s.n}
            delay={i * 0.08}
            className="relative mx-auto max-w-sm px-4 text-center"
          >
            <span className="text-faint absolute top-0 left-2 font-mono text-[10px]">{s.n}</span>
            <div className="border-border bg-card text-accent shadow-soft relative mx-auto mt-7 mb-5 grid size-[52px] place-items-center rounded-xl border">
              {s.icon}
            </div>
            <h3 className="mb-2 text-base font-semibold">{s.title}</h3>
            <p className="text-muted-foreground text-[13px]">{s.text}</p>
          </Reveal>
        ))}
      </ol>
    </section>
  );
}
