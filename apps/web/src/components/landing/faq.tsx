import { Accordion, AccordionItem } from "@/components/ui/accordion";
import { Reveal } from "@/components/ui/motion";
import { FAQS } from "./sample-data";
import { SectionIntro } from "./section-intro";

export function Faq() {
  return (
    <section id="faq" className="mx-auto w-full max-w-[820px] px-4 pb-24 sm:px-6 sm:pb-28">
      <SectionIntro kicker="Questions, answered" title="The details that matter." />
      <Reveal>
        <Accordion>
          {FAQS.map((f, i) => (
            <AccordionItem key={f.q} title={f.q} name="faq" defaultOpen={i === 0}>
              {f.a}
            </AccordionItem>
          ))}
        </Accordion>
      </Reveal>
    </section>
  );
}
