"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { createProjectsApi, type ProjectsApi } from "@/lib/projects-api";
import type { RepositoryPreview } from "@/lib/project-schema";
import { formatSize, projectFailureText } from "@/lib/project-text";

export function NewProjectForm({ csrf, api }: { csrf: string; api?: ProjectsApi }) {
  const router = useRouter();
  const [client] = useState(() => api ?? createProjectsApi());
  const [url, setUrl] = useState("");
  const [preview, setPreview] = useState<RepositoryPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function fail(f: Parameters<typeof projectFailureText>[0]) {
    if (f.kind === "unauthenticated") {
      router.replace("/login");
      return;
    }
    setError(projectFailureText(f));
  }

  async function check(event: FormEvent) {
    event.preventDefault();
    if (busy || url.trim() === "") return;
    setBusy(true);
    setError(null);
    setPreview(null);
    const res = await client.preview(csrf, url.trim());
    setBusy(false);
    if (res.ok) setPreview(res.repository);
    else fail(res);
  }

  async function createAndAnalyse() {
    if (busy) return;
    setBusy(true);
    setError(null);
    const created = await client.create(csrf, url.trim());
    if (!created.ok) {
      setBusy(false);
      fail(created);
      return;
    }
    // If starting the analysis fails the project still exists; its page offers to try again.
    await client.analyse(csrf, created.project.id);
    router.push(`/projects/${created.project.id}`);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Add a project</CardTitle>
        <p className="text-muted-foreground text-sm">
          Paste the address of a public GitHub repository, for example
          https://github.com/owner/repo.
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <form onSubmit={check} className="flex flex-col gap-3">
          <label htmlFor="repo-url" className="text-sm font-medium">
            Repository address
          </label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              id="repo-url"
              name="url"
              type="text"
              inputMode="url"
              autoComplete="off"
              spellCheck={false}
              maxLength={300}
              placeholder="https://github.com/owner/repo"
              value={url}
              onChange={(e) => {
                setUrl(e.target.value);
                setPreview(null);
                setError(null);
              }}
              aria-describedby="repo-error"
            />
            <Button type="submit" variant="secondary" disabled={busy || url.trim() === ""}>
              Check repository
            </Button>
          </div>
        </form>

        {preview ? (
          <div
            role="group"
            aria-label="Repository found"
            className="border-border flex flex-col gap-3 rounded-md border p-4"
          >
            <div className="flex flex-col gap-1">
              <span className="font-medium break-all">
                {preview.owner}/{preview.name}
              </span>
              <span className="text-muted-foreground text-sm">
                {preview.description || "No description."}
              </span>
              <span className="text-muted-foreground text-xs">
                Default branch {preview.default_branch} · {formatSize(preview.size_kb)}
              </span>
            </div>
            <div>
              <Button onClick={() => void createAndAnalyse()} disabled={busy}>
                Create and analyse
              </Button>
            </div>
          </div>
        ) : null}

        <p id="repo-error" role={error ? "alert" : undefined} className="text-bad text-sm">
          {error ?? ""}
        </p>
      </CardContent>
    </Card>
  );
}
