import type { Metadata } from "next";
import { AppHeader } from "@/components/app-header";
import { NewProjectForm } from "@/components/new-project-form";
import { requireUser } from "@/lib/require-user";

export const metadata: Metadata = { title: "New project · Readmely" };
export const dynamic = "force-dynamic";

export default async function NewProjectPage() {
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
  return (
    <main className="mx-auto flex max-w-2xl flex-col gap-6 p-6 sm:p-10">
      <AppHeader active="projects" email={user.me.user.email} credits={user.credits} />
      <h1 className="text-2xl font-semibold">New project</h1>
      <NewProjectForm csrf={user.me.csrf_token} />
    </main>
  );
}
