"use client";

import { RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { useProject } from "@/components/project/project-provider";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Code } from "@/components/ui/code";
import { ConfirmPanel } from "@/components/ui/confirm-panel";
import { Message } from "@/components/ui/message";
import { timeAgo } from "@/lib/format-time";
import { formatSize } from "@/lib/project-text";

/** Manage the project itself: read the repository again, or delete the project. */
export function SettingsTab() {
  const { project, working, busy, analyse, remove, error } = useProject();
  const [confirming, setConfirming] = useState(false);
  const analysed = project.job?.finished_at ?? null;

  return (
    <div className="flex max-w-[720px] flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Analyse again</CardTitle>
          <CardDescription>
            Reads the repository again to pick up new commits and replaces the analysis. The summary
            is written again, which costs a few credits.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="secondary" busy={busy} disabled={working} onClick={() => void analyse()}>
            <RefreshCw aria-hidden />
            {project.status === "failed" ? "Try again" : "Analyse again"}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Repository details</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
            <dt className="text-muted-foreground">Repository</dt>
            <dd className="break-all">
              <Code>{project.name}</Code>
            </dd>
            <dt className="text-muted-foreground">Default branch</dt>
            <dd>{project.default_branch || "unknown"}</dd>
            <dt className="text-muted-foreground">Size</dt>
            <dd>{formatSize(project.size_kb)}</dd>
            <dt className="text-muted-foreground">Last analysed</dt>
            <dd suppressHydrationWarning>{analysed ? timeAgo(analysed) : "Not yet"}</dd>
          </dl>
        </CardContent>
      </Card>

      <Card className="border-bad/30">
        <CardHeader>
          <CardTitle className="text-bad text-base">Delete project</CardTitle>
          <CardDescription>
            Removes the project and everything generated for it. The repository itself is not
            touched.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {confirming ? (
            <ConfirmPanel
              label="Confirm deleting this project"
              actions={
                <>
                  <Button variant="danger" busy={busy} onClick={() => void remove()}>
                    Yes, delete it
                  </Button>
                  <Button variant="secondary" onClick={() => setConfirming(false)} disabled={busy}>
                    Keep it
                  </Button>
                </>
              }
            >
              Delete {project.name}? This removes its analysis, README versions and diagram. This
              cannot be undone.
            </ConfirmPanel>
          ) : (
            <Button variant="danger" onClick={() => setConfirming(true)} disabled={busy}>
              <Trash2 aria-hidden /> Delete project
            </Button>
          )}
        </CardContent>
      </Card>

      {error ? <Message tone="error">{error}</Message> : null}
    </div>
  );
}
