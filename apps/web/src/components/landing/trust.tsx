import { FileText, Lock, Trash2 } from "lucide-react";
import { Reveal } from "@/components/ui/motion";
import { Kicker } from "./section-intro";

const ITEMS = [
  {
    icon: <Lock aria-hidden />,
    title: "Secrets stay secret",
    text: "Anything that looks like a secret is removed before storage and never shown. You see a count and file locations, nothing more.",
  },
  {
    icon: <FileText aria-hidden />,
    title: "AI output is labelled",
    text: "Generated text can be wrong. We say so wherever it appears, and every output is easy to edit.",
  },
  {
    icon: <Trash2 aria-hidden />,
    title: "Delete means delete",
    text: "Delete a project, or your whole account in one step, and its analysis and everything generated for it goes too.",
  },
];

export function Trust() {
  return (
    <section className="mx-auto grid w-full max-w-[1100px] gap-14 px-4 py-24 sm:px-6 sm:py-28 md:grid-cols-[0.9fr_1.1fr] md:gap-24">
      <Reveal className="flex flex-col items-start gap-4">
        <Kicker>Built for cautious developers</Kicker>
        <h2 className="text-[clamp(30px,4vw,46px)] leading-[1.12] font-semibold tracking-[-0.045em]">
          Your code deserves straight answers.
        </h2>
        <p className="text-muted-foreground max-w-md text-[15px]">
          Readmely reads public repositories only, and keeps the readable text of the files it
          analyses until you delete the project or your account.
        </p>
      </Reveal>
      <ul className="grid">
        {ITEMS.map((item, i) => (
          <Reveal
            as="li"
            key={item.title}
            delay={i * 0.07}
            className="border-border grid grid-cols-[42px_1fr] gap-4 border-b py-6 first:pt-0 last:border-b-0"
          >
            <span className="bg-accent-soft text-accent grid size-10 place-items-center rounded-lg [&>svg]:size-5">
              {item.icon}
            </span>
            <div>
              <h3 className="mb-1 text-sm font-semibold">{item.title}</h3>
              <p className="text-muted-foreground text-[13px]">{item.text}</p>
            </div>
          </Reveal>
        ))}
      </ul>
    </section>
  );
}
