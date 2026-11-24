import type { Metadata } from "next";
import { cookies } from "next/headers";
import { Faq } from "@/components/landing/faq";
import { FinalCta } from "@/components/landing/final-cta";
import { Hero } from "@/components/landing/hero";
import { HowItWorks } from "@/components/landing/how-it-works";
import { LandingHeader } from "@/components/landing/landing-header";
import { Outputs } from "@/components/landing/outputs";
import { Pricing } from "@/components/landing/pricing";
import { Sample } from "@/components/landing/sample";
import { Trust } from "@/components/landing/trust";
import { SiteFooter } from "@/components/layout/site-footer";
import { fetchSession, SESSION_COOKIE } from "@/lib/account-client";
import { loadApiPublicEnv, loadEnv } from "@/lib/env";
import { fetchStatus } from "@/lib/status-client";

export const metadata: Metadata = {
  title: "Readmely: a README, a context pack and a diagram from any repository",
};

// The footer shows live status and the header knows who is signed in: render per request.
export const dynamic = "force-dynamic";

/** Whether the cookie belongs to a live session. Any trouble reaching the API reads as "no". */
async function isSignedIn(token: string | undefined): Promise<boolean> {
  try {
    const session = await fetchSession(loadApiPublicEnv().apiPublicUrl, token);
    return session.kind === "signed_in";
  } catch {
    return false;
  }
}

export default async function Home() {
  const env = loadEnv();
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  const [signedIn, status] = await Promise.all([
    isSignedIn(token),
    fetchStatus(env.apiInternalUrl),
  ]);
  const health = status.kind === "report" ? status.report.status : null;

  return (
    <div className="overflow-x-clip">
      <a
        href="#main"
        className="bg-foreground text-background sr-only z-50 rounded-control px-3 py-2 text-sm focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
      >
        Skip to content
      </a>
      <LandingHeader signedIn={signedIn} />
      <main id="main">
        <Hero signedIn={signedIn} />
        <HowItWorks />
        <Outputs />
        <Sample />
        <Trust />
        <Pricing signedIn={signedIn} />
        <Faq />
        <FinalCta signedIn={signedIn} />
      </main>
      <SiteFooter health={health} />
    </div>
  );
}
