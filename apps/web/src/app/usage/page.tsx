import type { Metadata } from "next";
import { AppHeader } from "@/components/app-header";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { fetchCreditHistory } from "@/lib/account-client";
import { creditEntryLabel, creditEntryTone } from "@/lib/credit-text";
import { requireUser } from "@/lib/require-user";

export const metadata: Metadata = { title: "Usage · Readmely" };
export const dynamic = "force-dynamic";

export default async function UsagePage() {
  const user = await requireUser();
  if (user.unavailable) {
    return (
      <main className="mx-auto flex max-w-md flex-col gap-4 p-6 sm:p-10">
        <h1 className="text-2xl font-semibold">Usage unavailable</h1>
        <p role="alert" className="text-muted-foreground text-sm">
          We could not reach Readmely just now. Reload the page in a moment.
        </p>
      </main>
    );
  }
  const entries = await fetchCreditHistory(user.env.apiPublicUrl, user.token);

  return (
    <main className="mx-auto flex max-w-2xl flex-col gap-6 p-6 sm:p-10">
      <AppHeader active="usage" email={user.me.user.email} credits={user.credits} />
      <h1 className="text-2xl font-semibold">Usage</h1>

      <Card>
        <CardHeader>
          <CardTitle>Your credits</CardTitle>
          <p className="text-muted-foreground text-sm">
            Analysing and generating spend credits. Every new account starts with 100.
          </p>
        </CardHeader>
        <CardContent>
          {user.credits === null ? (
            <p className="text-muted-foreground text-sm">We could not load your balance.</p>
          ) : (
            <p className="text-3xl font-semibold" aria-label={`${user.credits} credits left`}>
              {user.credits.toLocaleString("en-US")}
              <span className="text-muted-foreground ml-2 text-sm font-normal">credits left</span>
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>History</CardTitle>
        </CardHeader>
        <CardContent>
          {entries === null ? (
            <p role="alert" className="text-muted-foreground text-sm">
              We could not load your history. Reload the page in a moment.
            </p>
          ) : entries.length === 0 ? (
            <p className="text-muted-foreground text-sm">Nothing yet.</p>
          ) : (
            <ul className="divide-border divide-y">
              {entries.map((e) => (
                <li key={e.id} className="flex items-center justify-between gap-3 py-2.5 text-sm">
                  <div className="flex flex-col">
                    <span>{creditEntryLabel(e)}</span>
                    <time dateTime={e.created_at} className="text-muted-foreground text-xs">
                      {new Date(e.created_at).toLocaleString("en-US", {
                        dateStyle: "medium",
                        timeStyle: "short",
                      })}
                    </time>
                  </div>
                  <Badge tone={creditEntryTone(e)}>
                    {e.delta > 0 ? "+" : ""}
                    {e.delta.toLocaleString("en-US")}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </main>
  );
}
