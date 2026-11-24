import type { Metadata } from "next";
import Link from "next/link";
import { LegalPage, type LegalSection } from "@/components/legal/legal-page";

export const metadata: Metadata = { title: "Terms of service · Readmely" };

const SECTIONS: LegalSection[] = [
  {
    id: "using-readmely",
    title: "Using Readmely",
    body: (
      <>
        <p>
          Readmely reads public GitHub repositories and writes documents about them. Private
          repositories are not supported. You may add up to 10 projects, and repositories over 200
          MB are refused.
        </p>
        <p>
          Only add repositories you are allowed to analyse. What Readmely produces describes the
          code it was given; the code itself stays under its own licence.
        </p>
      </>
    ),
  },
  {
    id: "your-account",
    title: "Your account",
    body: (
      <p>
        You sign in with an emailed one-time code, or with Google or GitHub. Keep your mailbox and
        those accounts secure: anyone who can use them can use your Readmely account. You can end
        any session from the account page, and delete the account at any time.
      </p>
    ),
  },
  {
    id: "credits",
    title: "Credits",
    body: (
      <>
        <p>
          Every new account receives 100 credits. AI work costs credits: Readmely holds the most a
          job can cost, charges what it actually used and returns the rest. A job that fails is
          refunded in full. Reading, editing and downloading never cost credits, and the context
          pack is free.
        </p>
        <p>
          Credits have no cash value and cannot be exchanged or transferred. Paid plans are not
          available yet; if they arrive, they will be described before you are asked to pay.
        </p>
      </>
    ),
  },
  {
    id: "generated-content",
    title: "What Readmely writes",
    body: (
      <>
        <p>
          READMEs, summaries, context packs and diagrams are written by software, partly by an AI
          model. They can be wrong or incomplete. <strong>Check them against the code</strong>{" "}
          before you publish or rely on them; you are responsible for what you do with them.
        </p>
        <p>
          Readmely does not claim ownership of what it generates for you. You can edit, copy and use
          it for your repositories.
        </p>
      </>
    ),
  },
  {
    id: "acceptable-use",
    title: "Acceptable use",
    body: (
      <ul>
        <li>Do not try to get around limits, credits or the security of the service.</li>
        <li>Do not use it to attack, overload or probe other people&apos;s systems.</li>
        <li>
          Do not put content into repositories meant to manipulate the AI into harmful output.
        </li>
        <li>Do not use another person&apos;s account.</li>
      </ul>
    ),
  },
  {
    id: "availability",
    title: "Availability",
    body: (
      <p>
        This is version 1. It can change, be slow, or be unavailable, and features may be added or
        removed. The{" "}
        <Link href="/status" className="text-accent underline">
          status page
        </Link>{" "}
        shows whether the services are running. Daily limits on AI work may pause generation for
        everyone; nothing is charged for work that does not run.
      </p>
    ),
  },
  {
    id: "ending",
    title: "Ending use",
    body: (
      <p>
        You can delete your account whenever you like, which removes your projects and everything
        generated for them. Accounts that abuse the service may be suspended.
      </p>
    ),
  },
  {
    id: "liability",
    title: "No warranty",
    body: (
      <p>
        Readmely is provided as it is, without promises about accuracy or uptime. To the extent the
        law allows, the people who run it are not liable for losses that come from using it,
        including from relying on text it wrote.
      </p>
    ),
  },
  {
    id: "changes-and-contact",
    title: "Changes and contact",
    body: (
      <p>
        If these terms change in a way that matters, the date at the top changes and the app says
        so. See also the{" "}
        <Link href="/privacy" className="text-accent underline">
          privacy policy
        </Link>
        . For questions, open an issue on the project&apos;s GitHub page, linked in the footer.
      </p>
    ),
  },
];

export default function TermsPage() {
  return (
    <LegalPage
      title="Terms of service"
      updated="24 November 2026"
      intro="Plain language, for version 1 of Readmely. By using the service you agree to them."
      sections={SECTIONS}
    />
  );
}
