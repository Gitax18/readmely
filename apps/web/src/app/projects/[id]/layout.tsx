import { notFound } from "next/navigation";
import { AppShell } from "@/components/layout/app-shell";
import { UnavailableScreen } from "@/components/layout/unavailable";
import { ProjectHeader } from "@/components/project/project-header";
import { ProjectProvider } from "@/components/project/project-provider";
import { Message } from "@/components/ui/message";
import { fetchProject } from "@/lib/account-client";
import { requireUser } from "@/lib/require-user";

export const dynamic = "force-dynamic";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * The frame around every tab of a project: the signed-in shell, the project header and the live
 * project state they share. The tabs themselves are the pages inside it.
 */
export default async function ProjectLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  if (!UUID.test(id)) notFound();
  const user = await requireUser();
  if (user.unavailable) return <UnavailableScreen />;
  const project = await fetchProject(user.env.apiPublicUrl, user.token, id);
  if (project === "missing") notFound();

  return (
    <AppShell
      active="projects"
      width="wide"
      email={user.me.user.email}
      credits={user.credits}
      csrf={user.me.csrf_token}
    >
      {project === null ? (
        <Message tone="error">
          We could not load this project just now. Reload the page in a moment.
        </Message>
      ) : (
        <ProjectProvider initial={project} csrf={user.me.csrf_token}>
          <ProjectHeader />
          {children}
        </ProjectProvider>
      )}
    </AppShell>
  );
}
