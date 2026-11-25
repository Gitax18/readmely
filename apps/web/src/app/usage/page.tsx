import type { Metadata } from "next";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { UnavailableScreen } from "@/components/layout/unavailable";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { Message } from "@/components/ui/message";
import { AnimatedNumber } from "@/components/ui/motion";
import { fetchCreditHistory } from "@/lib/account-client";
import { creditEntryLabel, creditEntryTone } from "@/lib/credit-text";
import { DIAGRAM_COST } from "@/lib/diagram-text";
import { requireUser } from "@/lib/require-user";
import { GENERATE_COST, SECTION_COST } from "@/lib/readme-text";
import { Coins } from "lucide-react";
import { ANALYSIS_COST, ANALYSIS_TYPICAL } from "@/lib/project-text";

export const metadata: Metadata = { title: "Usage · Readmely" };
export const dynamic = "force-dynamic";

/** What each action holds up front. What it really costs is usually less; the rest is returned. */
const COSTS: { action: string; cost: string; note: string }[] = [
  {
    action: "Analyse a repository",
    cost: `${ANALYSIS_COST} held, about ${ANALYSIS_TYPICAL} charged`,
    note: "Reads the code and writes the summary",
  },
  { action: "Write a README", cost: `Up to ${GENERATE_COST}`, note: "Includes a health check" },
  {
    action: "Rewrite a section",
    cost: `Up to ${SECTION_COST}`,
    note: "One heading of your README",
  },
  { action: "Draw a diagram", cost: `Up to ${DIAGRAM_COST}`, note: "SVG and PNG downloads" },
  { action: "Context pack", cost: "Free", note: "Built from the analysis" },
];

export default async function UsagePage() {
  const user = await requireUser();
  if (user.unavailable) return <UnavailableScreen title="Usage unavailable" />;
  const entries = await fetchCreditHistory(user.env.apiPublicUrl, user.token);

  return (
    <AppShell
      active="usage"
      email={user.me.user.email}
      credits={user.credits}
      csrf={user.me.csrf_token}
    >
      <PageHeader
        title="Usage"
        description="Analysing and generating spend credits. Every new account starts with 100."
      />

      <div className="grid gap-4 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <Card className="bg-navy border-navy-line text-white">
          <CardHeader className="pb-2">
            <CardTitle className="text-navy-muted text-sm font-semibold">Credits left</CardTitle>
          </CardHeader>
          <CardContent>
            {user.credits === null ? (
              <p className="text-navy-muted text-sm">We could not load your balance.</p>
            ) : (
              <p className="flex items-baseline gap-2" aria-label={`${user.credits} credits left`}>
                <AnimatedNumber
                  value={user.credits}
                  className="font-mono text-5xl font-medium tracking-tight"
                />
                <span className="text-navy-muted text-sm">credits</span>
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">What things cost</CardTitle>
          </CardHeader>
          <CardContent>
            <table className="w-full text-sm">
              <caption className="sr-only">Credit cost of each action</caption>
              <thead className="sr-only">
                <tr>
                  <th scope="col">Action</th>
                  <th scope="col">Cost</th>
                </tr>
              </thead>
              <tbody className="divide-border divide-y">
                {COSTS.map((c) => (
                  <tr key={c.action}>
                    <th scope="row" className="py-2 pr-3 text-left font-medium">
                      {c.action}
                      <span className="text-muted-foreground block text-xs font-normal">
                        {c.note}
                      </span>
                    </th>
                    <td className="py-2 text-right font-mono text-xs whitespace-nowrap">
                      {c.cost}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">History</CardTitle>
          <CardDescription>Every change to your balance, newest first.</CardDescription>
        </CardHeader>
        <CardContent>
          {entries === null ? (
            <Message tone="error">
              We could not load your history. Reload the page in a moment.
            </Message>
          ) : entries.length === 0 ? (
            <EmptyState icon={<Coins aria-hidden />} title="Nothing yet">
              Your credit activity shows up here once you analyse or generate something.
            </EmptyState>
          ) : (
            <ul className="divide-border divide-y">
              {entries.map((e) => (
                <li key={e.id} className="flex items-center justify-between gap-3 py-3 text-sm">
                  <div className="flex min-w-0 flex-col">
                    <span>{creditEntryLabel(e)}</span>
                    <time dateTime={e.created_at} className="text-muted-foreground text-xs">
                      {new Date(e.created_at).toLocaleString("en-US", {
                        dateStyle: "medium",
                        timeStyle: "short",
                      })}
                    </time>
                  </div>
                  <Badge tone={creditEntryTone(e)} className="font-mono">
                    {e.delta > 0 ? "+" : ""}
                    {e.delta.toLocaleString("en-US")}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </AppShell>
  );
}
