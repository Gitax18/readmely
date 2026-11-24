import type { Metadata } from "next";
import Link from "next/link";
import { LegalPage, type LegalSection } from "@/components/legal/legal-page";

export const metadata: Metadata = { title: "Privacy policy · Readmely" };

const SECTIONS: LegalSection[] = [
  {
    id: "what-this-covers",
    title: "What this covers",
    body: (
      <p>
        Readmely reads a public GitHub repository you point it at and writes a README, a context
        pack and an architecture diagram from it. This page says what the service stores about you
        and your projects, who else receives any of it, and how to remove it.
      </p>
    ),
  },
  {
    id: "what-we-store",
    title: "What we store",
    body: (
      <>
        <p>
          <strong>Your account.</strong> Your email address and, if you set one, a display name. If
          you sign in with Google or GitHub, the email and account identifier that provider gives
          us. There are no passwords.
        </p>
        <p>
          <strong>Your sign-ins.</strong> For each signed-in device: a short description of the
          browser and system, the IP address it signed in from, and when. The account page lists
          them and lets you end any of them.
        </p>
        <p>
          <strong>Your projects.</strong> For each repository: its address, name, description,
          default branch and size, and what the analysis found. The analysis includes the{" "}
          <strong>readable text of the files it read</strong> (up to about 20,000 files), the file
          structure, languages, commands and services.
        </p>
        <p>
          <strong>What is generated.</strong> The summary, every version of the README, and the
          diagram.
        </p>
        <p>
          <strong>Your credits.</strong> A ledger of credits granted, held, returned and refunded,
          and a record of each AI call (the model used, the amount of text and its cost).
        </p>
      </>
    ),
  },
  {
    id: "secrets",
    title: "Secrets in repositories",
    body: (
      <p>
        If the analysis finds something that looks like a secret (a key, a token, a password), it is
        removed before anything is stored and before any text reaches an AI model. Readmely keeps
        only a count and the file and line where it was found, never the value. If a real secret was
        committed, rotate it: it is still in the repository&apos;s history.
      </p>
    ),
  },
  {
    id: "what-we-dont-do",
    title: "What we do not do",
    body: (
      <ul>
        <li>No advertising, no tracking pixels and no third-party analytics.</li>
        <li>
          The only things set in your browser are the sign-in cookie and your light or dark theme
          choice.
        </li>
        <li>We do not sell or share your data for marketing.</li>
        <li>There are no payments yet, so no payment details are collected.</li>
      </ul>
    ),
  },
  {
    id: "who-receives-it",
    title: "Who else receives data",
    body: (
      <>
        <p>
          <strong>An AI model provider (OpenAI).</strong> To write summaries, READMEs and diagrams,
          text taken from your repository, with secrets already removed, is sent to the provider.
          Repository text is treated as data to read, never as instructions.
        </p>
        <p>
          <strong>An email relay.</strong> Your address and the message go through an SMTP relay so
          that sign-in codes and alerts reach you.
        </p>
        <p>
          <strong>GitHub.</strong> Readmely calls GitHub&apos;s public API and downloads the public
          repository you named. If you sign in with Google or GitHub, that provider learns that you
          signed in.
        </p>
        <p>The service runs on a hosting provider, which stores the data described above.</p>
      </>
    ),
  },
  {
    id: "keeping-and-deleting",
    title: "Keeping and deleting",
    body: (
      <>
        <p>
          Everything is kept until you remove it. <strong>Delete a project</strong> and its
          analysis, README versions and diagram go with it. <strong>Delete your account</strong> (it
          asks for a code we email you) and the account, every project and everything generated are
          removed, and you are signed out everywhere.
        </p>
        <p>
          Credit ledger entries are kept in anonymised form, so the accounts balance, but they no
          longer point to you. Sign-in sessions end after 7 days without use, or 30 days after you
          signed in.
        </p>
      </>
    ),
  },
  {
    id: "your-choices",
    title: "Your choices",
    body: (
      <p>
        You can see your account details, your signed-in devices and your credit history in the app,
        rename yourself, disconnect Google or GitHub, end any session, and delete your account. For
        anything else, open an issue on the project&apos;s GitHub page, linked in the footer.
      </p>
    ),
  },
  {
    id: "changes",
    title: "Changes",
    body: (
      <p>
        If this policy changes in a way that matters, the date at the top changes and the app says
        so. The terms of use are on the{" "}
        <Link href="/terms" className="text-accent underline">
          terms page
        </Link>
        .
      </p>
    ),
  },
];

export default function PrivacyPage() {
  return (
    <LegalPage
      title="Privacy policy"
      updated="24 November 2026"
      intro="Plain language, for version 1 of Readmely. It describes what the service does today."
      sections={SECTIONS}
    />
  );
}
