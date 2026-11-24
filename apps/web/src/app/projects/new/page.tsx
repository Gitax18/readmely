import type { Metadata } from "next";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { UnavailableScreen } from "@/components/layout/unavailable";
import { NewProjectForm } from "@/components/new-project-form";
import { repoParam } from "@/lib/github-url";
import { requireUser } from "@/lib/require-user";

export const metadata: Metadata = { title: "New project · Readmely" };
export const dynamic = "force-dynamic";

export default async function NewProjectPage({
  searchParams,
}: {
  searchParams: Promise<{ repo?: string | string[] }>;
}) {
  const user = await requireUser();
  if (user.unavailable) return <UnavailableScreen />;
  const repo = repoParam((await searchParams).repo);
  return (
    <AppShell
      active="projects"
      width="narrow"
      email={user.me.user.email}
      credits={user.credits}
      csrf={user.me.csrf_token}
    >
      <PageHeader title="New project" back={{ href: "/dashboard", label: "Projects" }} />
      <NewProjectForm csrf={user.me.csrf_token} initialUrl={repo ?? ""} />
    </AppShell>
  );
}
