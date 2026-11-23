import type { Metadata } from "next";
import Link from "next/link";
import { AppHeader } from "@/components/app-header";
import { ProjectsView } from "@/components/projects-view";
import { fetchProjects } from "@/lib/account-client";
import { requireUser } from "@/lib/require-user";

export const metadata: Metadata = { title: "Projects · Readmely" };
export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  const user = await requireUser();
  if (user.unavailable) return <Unavailable />;
  const projects = await fetchProjects(user.env.apiPublicUrl, user.token);

  return (
    <main className="mx-auto flex max-w-4xl flex-col gap-6 p-6 sm:p-10">
      <AppHeader active="projects" email={user.me.user.email} />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold">Your projects</h1>
        {projects && projects.length > 0 ? (
          <Link
            href="/projects/new"
            className="bg-foreground text-background inline-flex h-10 items-center rounded-md px-4 text-sm font-medium hover:opacity-90"
          >
            New project
          </Link>
        ) : null}
      </div>
      {projects === null ? (
        <p role="alert" className="text-muted-foreground text-sm">
          We could not load your projects just now. Reload the page in a moment.
        </p>
      ) : (
        <ProjectsView initial={projects} />
      )}
    </main>
  );
}

function Unavailable() {
  return (
    <main className="mx-auto flex max-w-md flex-col gap-4 p-6 sm:p-10">
      <h1 className="text-2xl font-semibold">Projects unavailable</h1>
      <p role="alert" className="text-muted-foreground text-sm">
        We could not reach Readmely just now. Reload the page in a moment.
      </p>
    </main>
  );
}
