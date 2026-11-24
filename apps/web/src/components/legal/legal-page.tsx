import { PublicHeader } from "@/components/layout/public-header";
import { SiteFooter } from "@/components/layout/site-footer";

export type LegalSection = { id: string; title: string; body: React.ReactNode };

/** A long reading page: title, date, a contents list beside the text on wide screens. */
export function LegalPage({
  title,
  updated,
  intro,
  sections,
}: {
  title: string;
  updated: string;
  intro: React.ReactNode;
  sections: LegalSection[];
}) {
  return (
    <div className="flex min-h-screen flex-col">
      <PublicHeader />
      <main
        id="main"
        className="mx-auto grid w-full max-w-[1000px] flex-1 gap-10 px-4 py-10 sm:px-6 lg:grid-cols-[220px_1fr] lg:gap-16"
      >
        <header className="animate-fade-up lg:col-span-2">
          <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">{title}</h1>
          <p className="text-muted-foreground mt-2 text-sm">Last updated {updated}</p>
          <p className="text-muted-foreground mt-4 max-w-[70ch] text-[15px] leading-relaxed">
            {intro}
          </p>
        </header>
        <nav aria-label="Contents" className="lg:sticky lg:top-8 lg:self-start">
          <details className="group lg:open:block" open>
            <summary className="text-muted-foreground mb-3 list-none cursor-pointer [&::-webkit-details-marker]:hidden text-xs font-bold tracking-[0.1em] uppercase lg:cursor-default">
              Contents
            </summary>
            <ol className="text-muted-foreground flex flex-col gap-2 text-[13px]">
              {sections.map((s, i) => (
                <li key={s.id}>
                  <a href={`#${s.id}`} className="hover:text-foreground transition-colors">
                    {i + 1}. {s.title}
                  </a>
                </li>
              ))}
            </ol>
          </details>
        </nav>
        <div className="flex max-w-[70ch] flex-col gap-10">
          {sections.map((s, i) => (
            <section key={s.id} id={s.id} aria-labelledby={`${s.id}-h`}>
              <h2 id={`${s.id}-h`} className="mb-3 text-xl font-semibold tracking-tight">
                {i + 1}. {s.title}
              </h2>
              <div className="text-muted-foreground flex flex-col gap-3 text-[15px] leading-relaxed [&_li]:ml-5 [&_li]:list-disc [&_strong]:text-foreground">
                {s.body}
              </div>
            </section>
          ))}
        </div>
      </main>
      <SiteFooter compact />
    </div>
  );
}
