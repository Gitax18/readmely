import { CenteredShell } from "@/components/layout/app-shell";
import { Wordmark } from "@/components/ui/logo";

/** What a signed-in page shows when the API cannot be reached: not "signed out", just "not now". */
export function UnavailableScreen({ title = "Unavailable" }: { title?: string }) {
  return (
    <CenteredShell>
      <Wordmark />
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p role="alert" className="text-muted-foreground text-sm">
        We could not reach Readmely just now. Reload the page in a moment.
      </p>
    </CenteredShell>
  );
}
