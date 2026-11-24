import type { Metadata } from "next";
import Link from "next/link";
import { Plus } from "lucide-react";
import { LOW_CREDITS } from "@/components/app-header";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { ProjectsView } from "@/components/projects-view";
import { Button, buttonVariants } from "@/components/ui/button";
import { Message } from "@/components/ui/message";
import { fetchProjects } from "@/lib/account-client";
import { requireUser } from "@/lib/require-user";
import { UnavailableScreen } from "@/components/layout/unavailable";

export const metadata: Metadata = { title: "Projects · Readmely" };
export const dynamic = "force-dynamic";

/** The most projects one account can hold. */
const PROJECT_LIMIT = 10;

export default async function DashboardPage() {
  const user = await requireUser();
  if (user.unavailable) return <UnavailableScreen title="Projects unavailable" />;
  const projects = await fetchProjects(user.env.apiPublicUrl, user.token);
  const count = projects?.length ?? 0;
  const full = count >= PROJECT_LIMIT;

  return (
    <AppShell
      active="projects"
      email={user.me.user.email}
      credits={user.credits}
      csrf={user.me.csrf_token}
    >
      <PageHeader
        title="Projects"
        description={projects ? `${count} of ${PROJECT_LIMIT} projects` : undefined}
        actions={
          full ? (
            <Button disabled>
              <Plus aria-hidden /> New project
            </Button>
          ) : projects && projects.length > 0 ? (
            <Link href="/projects/new" className={buttonVariants({ variant: "primary" })}>
              <Plus aria-hidden /> New project
            </Link>
          ) : null
        }
      />
      {full ? (
        <Message tone="info">
          You have {PROJECT_LIMIT} projects, the most for now. Delete one to add another.
        </Message>
      ) : null}
      {typeof user.credits === "number" && user.credits < LOW_CREDITS ? (
        <Message tone="warn">
          {user.credits === 0
            ? "You are out of credits. "
            : `You have ${user.credits} credit${user.credits === 1 ? "" : "s"} left. `}
          <Link href="/usage" className="font-semibold underline underline-offset-2">
            See what each action costs
          </Link>
        </Message>
      ) : null}
      {projects === null ? (
        <Message tone="error">
          We could not load your projects just now. Reload the page in a moment.
        </Message>
      ) : (
        <ProjectsView initial={projects} />
      )}
    </AppShell>
  );
}
