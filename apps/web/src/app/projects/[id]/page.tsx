import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { AppHeader } from "@/components/app-header";
import { ProjectView } from "@/components/project-view";
import { fetchProject } from "@/lib/account-client";
import { requireUser } from "@/lib/require-user";

export const metadata: Metadata = { title: "Project · Readmely" };
export const dynamic = "force-dynamic";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export default async function ProjectPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  if (!UUID.test(id)) notFound();
  const user = await requireUser();
  if (user.unavailable) {
    return (
      <main className="mx-auto flex max-w-md flex-col gap-4 p-6 sm:p-10">
        <h1 className="text-2xl font-semibold">Unavailable</h1>
        <p role="alert" className="text-muted-foreground text-sm">
          We could not reach Readmely just now. Reload the page in a moment.
        </p>
      </main>
    );
  }
  const project = await fetchProject(user.env.apiPublicUrl, user.token, id);
  if (project === "missing") notFound();
  return (
    <main className="mx-auto flex max-w-3xl flex-col gap-6 p-6 sm:p-10">
      <AppHeader active="projects" email={user.me.user.email} credits={user.credits} />
      {project === null ? (
        <p role="alert" className="text-muted-foreground text-sm">
          We could not load this project just now. Reload the page in a moment.
        </p>
      ) : (
        <ProjectView initial={project} csrf={user.me.csrf_token} />
      )}
    </main>
  );
}
