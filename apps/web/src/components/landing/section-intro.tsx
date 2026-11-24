import { cn } from "@/lib/utils";

export function Kicker({ className, children }: { className?: string; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "text-accent inline-flex items-center gap-2 text-[11px] font-bold tracking-[0.12em] uppercase",
        className,
      )}
    >
      {children}
    </span>
  );
}

/** Kicker, heading and lead for one landing section. */
export function SectionIntro({
  kicker,
  title,
  children,
  align = "center",
  className,
}: {
  kicker: string;
  title: React.ReactNode;
  children?: React.ReactNode;
  align?: "center" | "left";
  className?: string;
}) {
  return (
    <div
      className={cn(
        "mb-14 flex max-w-2xl flex-col gap-4",
        align === "center" ? "mx-auto items-center text-center" : "items-start text-left",
        className,
      )}
    >
      <Kicker>{kicker}</Kicker>
      <h2 className="text-[clamp(30px,4vw,46px)] text-balance leading-[1.12] font-semibold tracking-[-0.045em]">
        {title}
      </h2>
      {children ? <p className="text-muted-foreground max-w-xl text-[15px]">{children}</p> : null}
    </div>
  );
}
